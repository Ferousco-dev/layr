package assets

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

const (
	maxRedirects = 3
	// HeadBytes is how much of a file is kept in memory to identify it and read its dimensions.
	HeadBytes = 1 << 20
)

// blockedRanges are destinations an asset URL must never reach.
var blockedRanges = mustPrefixes(
	"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12",
	"192.0.0.0/24", "192.168.0.0/16", "198.18.0.0/15", "224.0.0.0/4", "240.0.0.0/4",
	"::/128", "::1/128", "fc00::/7", "fe80::/10", "ff00::/8", "fd00:ec2::254/128",
)

func mustPrefixes(cidrs ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(cidrs))
	for i, c := range cidrs {
		out[i] = netip.MustParsePrefix(c)
	}
	return out
}

// blockedIP reports whether ip is loopback, private, link-local, metadata or otherwise internal.
func blockedIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	for _, p := range blockedRanges {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// Policy decides which URLs and connections are acceptable.
type Policy struct {
	// AllowInsecure permits http and internal addresses; only tests set it.
	AllowInsecure bool
}

// validateURL checks the parts of a URL that can be judged before connecting.
func (p Policy) validateURL(u *url.URL) error {
	if u == nil || u.User != nil || u.Host == "" || u.Opaque != "" {
		return ErrURLBlocked
	}
	switch {
	case u.Scheme == "https":
	case u.Scheme == "http" && p.AllowInsecure:
	default:
		return ErrURLBlocked
	}
	if !p.AllowInsecure {
		if port := u.Port(); port != "" && port != "443" {
			return ErrURLBlocked
		}
		if ip, err := netip.ParseAddr(strings.Trim(u.Hostname(), "[]")); err == nil && blockedIP(ip) {
			return ErrURLBlocked
		}
		if h := strings.ToLower(u.Hostname()); h == "localhost" || strings.HasSuffix(h, ".localhost") || strings.HasSuffix(h, ".internal") || strings.HasSuffix(h, ".local") {
			return ErrURLBlocked
		}
	}
	return nil
}

// control refuses internal addresses at connect time, after DNS, redirects and rebinding.
func (p Policy) control(_, address string, _ syscall.RawConn) error {
	if p.AllowInsecure {
		return nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return ErrURLBlocked
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || blockedIP(ip) {
		return ErrURLBlocked
	}
	return nil
}

// Downloader fetches provider-issued URLs with size, redirect and destination limits.
type Downloader struct {
	client   *http.Client
	policy   Policy
	maxBytes int64
}

// NewDownloader builds the shared client; proxies from the environment are deliberately ignored.
func NewDownloader(policy Policy, maxBytes int64) *Downloader {
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: policy.control}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		MaxIdleConns:          8,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       30 * time.Second,
	}
	d := &Downloader{policy: policy, maxBytes: maxBytes}
	d.client = &http.Client{
		Transport: transport,
		Timeout:   2 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return ErrURLBlocked
			}
			return policy.validateURL(req.URL)
		},
	}
	return d
}

// Fetched describes a completed download.
type Fetched struct {
	ContentType string
	Size        int64
}

// StatusError is a non-200 answer from the asset host.
type StatusError struct{ Status int }

func (e *StatusError) Error() string { return fmt.Sprintf("asset host answered %d", e.Status) }

// Fetch streams the body of rawURL into dst, never holding more than the size limit.
func (d *Downloader) Fetch(ctx context.Context, rawURL string, dst io.Writer) (Fetched, error) {
	u, err := url.Parse(rawURL)
	if err != nil || d.policy.validateURL(u) != nil {
		return Fetched{}, ErrURLBlocked
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Fetched{}, ErrURLBlocked
	}
	req.Header.Set("User-Agent", "Layr/0.1")

	resp, err := d.client.Do(req)
	if err != nil {
		return Fetched{}, clean(ctx, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return Fetched{}, &StatusError{Status: resp.StatusCode}
	}
	if resp.ContentLength > d.maxBytes {
		return Fetched{}, ErrTooLarge
	}

	n, err := io.Copy(dst, io.LimitReader(resp.Body, d.maxBytes+1))
	if err != nil {
		return Fetched{}, clean(ctx, err)
	}
	if n > d.maxBytes {
		return Fetched{}, ErrTooLarge
	}
	return Fetched{ContentType: resp.Header.Get("Content-Type"), Size: n}, nil
}

// clean strips the URL from transport errors, since signed URLs must not reach logs.
func clean(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	if errors.Is(err, ErrURLBlocked) {
		return ErrURLBlocked
	}
	if errors.Is(err, ErrTooLarge) || errors.Is(err, ErrBudgetExceeded) {
		return err
	}
	return fmt.Errorf("%w: %s", ErrDownload, errorKind(err))
}

func errorKind(err error) string {
	var ne net.Error
	switch {
	case errors.As(err, &ne) && ne.Timeout():
		return "timeout"
	case errors.Is(err, io.ErrUnexpectedEOF):
		return "truncated"
	}
	return "network"
}
