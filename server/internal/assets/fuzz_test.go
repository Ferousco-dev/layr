package assets

import (
	"bytes"
	"encoding/xml"
	"io"
	"regexp"
	"strings"
	"testing"
)

var safeFileName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

func FuzzFileNameIsAlwaysSafe(f *testing.F) {
	for _, s := range []string{"Logo", "../../etc/passwd", `..\x`, "日本語", "a\x00b", strings.Repeat("a", 500), "%2e%2e/", "CON", ".hidden"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, name string) {
		got := fileName(name, "vector", "a81f2cdeadbeef", "svg", 6)
		if !safeFileName.MatchString(got) || strings.Contains(got, "..") {
			t.Fatalf("fileName(%q) = %q", name, got)
		}
	})
}

func FuzzSanitizeSVGNeverPanicsAndLeavesNoActiveContent(f *testing.F) {
	for _, s := range []string{
		`<svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0"/></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg" onload="x()"><a href="javascript:x()"/></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg"><foreignObject><div/></foreignObject></svg>`,
		`<!DOCTYPE svg [<!ENTITY x "y">]><svg xmlns="http://www.w3.org/2000/svg">&x;</svg>`,
		``, `<`, `<svg`,
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, doc []byte) {
		out, _, err := sanitizeSVG(doc)
		if err != nil {
			return
		}
		assertNoActiveContent(t, out)
	})
}

func FuzzSniffNeverPanics(f *testing.F) {
	f.Add([]byte("\x89PNG\r\n\x1a\n"))
	f.Add([]byte("<svg"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, head []byte) {
		sniff(head)
		looksLikeSVG(head)
	})
}

// assertNoActiveContent walks the tokens: text may say anything, but elements and attributes may not be active.
func assertNoActiveContent(t *testing.T, out []byte) {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(out))
	dec.Strict = false
	for {
		tok, err := dec.RawToken()
		if err == io.EOF || err != nil {
			return
		}
		switch e := tok.(type) {
		case xml.StartElement:
			switch strings.ToLower(e.Name.Local) {
			case "script", "foreignobject", "iframe", "object", "embed":
				t.Fatalf("active element %q survived: %q", e.Name.Local, out)
			}
			for _, a := range e.Attr {
				name, val := strings.ToLower(a.Name.Local), strings.ToLower(strings.TrimSpace(a.Value))
				if strings.HasPrefix(name, "on") {
					t.Fatalf("event handler %q survived: %q", a.Name.Local, out)
				}
				if name == "href" && (strings.HasPrefix(val, "javascript:") || strings.HasPrefix(val, "data:") || strings.Contains(val, "://")) {
					t.Fatalf("unsafe href %q survived: %q", a.Value, out)
				}
			}
		case xml.Directive:
			if bytes.Contains(bytes.ToLower(e), []byte("entity")) {
				t.Fatalf("entity declaration survived: %q", out)
			}
		}
	}
}
