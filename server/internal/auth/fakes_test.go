package auth

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ferousco-dev/layr/server/internal/figma"
	"github.com/ferousco-dev/layr/server/internal/oauthstate"
)

var t0 = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

type fakeStore struct {
	mu       sync.Mutex
	users    map[string]User
	sessions map[string]*SessionRecord
	hashes   [][]byte
	conn     Connection
	saves    int
}

func newStore() *fakeStore {
	return &fakeStore{users: map[string]User{}, sessions: map[string]*SessionRecord{}}
}

func (f *fakeStore) UpsertLogin(_ context.Context, l Login) (User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[l.Identity.ID]
	if !ok {
		u.ID = "user-" + l.Identity.ID
	}
	u.FigmaUserID, u.Email, u.DisplayName, u.AvatarURL = l.Identity.ID, l.Identity.Email, l.Identity.DisplayName, l.Identity.AvatarURL
	f.users[l.Identity.ID] = u
	f.conn = Connection{ID: "conn-1", UserID: u.ID, AccessCT: l.AccessCT, RefreshCT: l.RefreshCT, ExpiresAt: l.ExpiresAt, Status: StatusActive}
	return u, nil
}

func (f *fakeStore) CreateSession(_ context.Context, userID string, hash []byte, expires, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	var user User
	for _, u := range f.users {
		if u.ID == userID {
			user = u
		}
	}
	f.sessions[hex.EncodeToString(hash)] = &SessionRecord{ID: "s-" + userID, User: user, ExpiresAt: expires, LastSeenAt: now}
	f.hashes = append(f.hashes, hash)
	return nil
}

func (f *fakeStore) SessionByHash(_ context.Context, hash []byte) (SessionRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.sessions[hex.EncodeToString(hash)]
	if !ok {
		return SessionRecord{}, ErrNotFound
	}
	return *r, nil
}

func (f *fakeStore) TouchSession(_ context.Context, id string, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.sessions {
		if r.ID == id {
			r.LastSeenAt = now
		}
	}
	return nil
}

func (f *fakeStore) RevokeSession(_ context.Context, hash []byte, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.sessions[hex.EncodeToString(hash)]; ok && r.RevokedAt == nil {
		r.RevokedAt = &now
	}
	return nil
}

func (f *fakeStore) Connection(context.Context, string) (Connection, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.conn.ID == "" {
		return Connection{}, ErrNotFound
	}
	return f.conn, nil
}

func (f *fakeStore) SaveTokens(_ context.Context, _ string, access, refresh []byte, expires, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.conn.AccessCT, f.conn.ExpiresAt = access, expires
	if refresh != nil {
		f.conn.RefreshCT = refresh
	}
	f.saves++
	return nil
}

func (f *fakeStore) MarkReconnectRequired(context.Context, string, time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.conn.Status = StatusReconnectNeed
	return nil
}

type fakeFigma struct {
	exchangeErr, meErr, refreshErr error
	tokens                         figma.Tokens
	identity                       figma.Identity
	refreshTokens                  figma.Tokens
	refreshCalls                   atomic.Int32
	refreshDelay                   time.Duration
}

func (f *fakeFigma) AuthorizeURL(state, challenge string) string {
	return "https://figma.test/oauth?state=" + state + "&code_challenge=" + challenge
}

func (f *fakeFigma) Exchange(context.Context, string, string) (figma.Tokens, error) {
	return f.tokens, f.exchangeErr
}

func (f *fakeFigma) Refresh(context.Context, string) (figma.Tokens, error) {
	f.refreshCalls.Add(1)
	time.Sleep(f.refreshDelay)
	return f.refreshTokens, f.refreshErr
}

func (f *fakeFigma) Me(context.Context, string) (figma.Identity, error) {
	return f.identity, f.meErr
}

type fakeStates struct {
	mu    sync.Mutex
	live  map[string]string
	count int
}

func newStates() *fakeStates { return &fakeStates{live: map[string]string{}} }

func (f *fakeStates) Begin(context.Context) (oauthstate.Attempt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.count++
	state := "state-" + string(rune('a'+f.count))
	f.live[state] = "verifier"
	return oauthstate.Attempt{State: state, Challenge: "challenge"}, nil
}

func (f *fakeStates) Consume(_ context.Context, state string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.live[state]
	delete(f.live, state)
	if !ok {
		return "", oauthstate.ErrInvalid
	}
	return v, nil
}

type fakeLocker struct {
	mu   sync.Mutex
	held map[string]string
}

func newLocker() *fakeLocker { return &fakeLocker{held: map[string]string{}} }

func (l *fakeLocker) Acquire(_ context.Context, key string, _ time.Duration) (string, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, taken := l.held[key]; taken {
		return "", false, nil
	}
	l.held[key] = "owner"
	return "owner", true, nil
}

func (l *fakeLocker) Release(_ context.Context, key, owner string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.held[key] == owner {
		delete(l.held, key)
	}
	return nil
}

// xorSealer is a reversible stand-in; real AES-GCM is tested in the credential package.
type xorSealer struct{}

func (xorSealer) Seal(p []byte, purpose string) ([]byte, error) {
	return append([]byte(purpose+":"), p...), nil
}

func (xorSealer) Open(s []byte, purpose string) ([]byte, error) {
	prefix := []byte(purpose + ":")
	if !bytes.HasPrefix(s, prefix) {
		return nil, errors.New("wrong purpose")
	}
	return s[len(prefix):], nil
}

type harness struct {
	svc    *Service
	store  *fakeStore
	figma  *fakeFigma
	states *fakeStates
	clock  *time.Time
}

func newHarness() *harness {
	now := t0
	h := &harness{store: newStore(), states: newStates(), clock: &now}
	h.figma = &fakeFigma{
		tokens:   figma.Tokens{Access: "acc", Refresh: "ref", ExpiresAt: t0.Add(time.Hour), UserID: "fig-1"},
		identity: figma.Identity{ID: "fig-1", Email: "a@b.c", DisplayName: "Ada"},
	}
	h.svc = New(Deps{
		Store: h.store, Figma: h.figma, States: h.states, Sealer: xorSealer{}, Locker: newLocker(),
		SessionTTL: 24 * time.Hour, Now: func() time.Time { return *h.clock },
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	h.svc.lockWait, h.svc.lockPoll = time.Second, 5*time.Millisecond
	return h
}
