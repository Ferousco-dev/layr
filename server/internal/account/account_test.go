package account

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/credential"
)

type memStore struct {
	mu      sync.Mutex
	keys    map[string]map[string][]byte // user -> provider -> ciphertext
	hints   map[string]string
	imports map[string][]string
	users   map[string]bool
}

func newMem() *memStore {
	return &memStore{keys: map[string]map[string][]byte{}, hints: map[string]string{}, imports: map[string][]string{}, users: map[string]bool{"u1": true, "u2": true}}
}

func (m *memStore) Keys(_ context.Context, user string) ([]KeyStatus, error) {
	var out []KeyStatus
	for p := range m.keys[user] {
		out = append(out, KeyStatus{Provider: p, Saved: true, Hint: m.hints[user+p]})
	}
	return out, nil
}
func (m *memStore) SaveKey(_ context.Context, user, provider string, ct []byte, hint string, now time.Time) (KeyStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.keys[user] == nil {
		m.keys[user] = map[string][]byte{}
	}
	m.keys[user][provider], m.hints[user+provider] = ct, hint
	return KeyStatus{Provider: provider, Saved: true, Hint: hint, UpdatedAt: now}, nil
}
func (m *memStore) DeleteKey(_ context.Context, user, provider string) (bool, error) {
	_, ok := m.keys[user][provider]
	delete(m.keys[user], provider)
	return ok, nil
}
func (m *memStore) KeyCiphertext(_ context.Context, user, provider string) ([]byte, error) {
	if ct, ok := m.keys[user][provider]; ok {
		return ct, nil
	}
	return nil, ErrKeyNotFound
}
func (m *memStore) FigmaStatus(context.Context, string) (string, error) { return "active", nil }
func (m *memStore) ImportIDs(_ context.Context, user string) ([]string, error) {
	return m.imports[user], nil
}
func (m *memStore) DeleteUser(_ context.Context, user string) (bool, error) {
	ok := m.users[user]
	delete(m.users, user)
	delete(m.keys, user)
	return ok, nil
}

type spyFiles struct{ cleaned []string }

func (s *spyFiles) Cleanup(id string) error { s.cleaned = append(s.cleaned, id); return nil }

const goodAnthropic = "sk-ant-api03-abcdefghijklmnopqrstuvwxyz0123456789"

func newService(t *testing.T) (*Service, *memStore, *spyFiles, *bytes.Buffer) {
	t.Helper()
	sealer, err := credential.New(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	store, files, logs := newMem(), &spyFiles{}, &bytes.Buffer{}
	return New(store, sealer, files, slog.New(slog.NewTextHandler(logs, nil))), store, files, logs
}

func TestValidateKeyChecksShapeNotTruth(t *testing.T) {
	cases := []struct {
		provider, key string
		want          error
	}{
		{Anthropic, goodAnthropic, nil},
		{OpenAI, "sk-proj-abcdefghijklmnopqrstuvwxyz012345", nil},
		{XAI, "xai-abcdefghijklmnopqrstuvwxyz0123456789", nil},
		{Anthropic, "sk-proj-abcdefghijklmnopqrstuvwxyz012345", ErrInvalidKey},
		{OpenAI, "xai-abcdefghijklmnopqrstuvwxyz0123456789", ErrInvalidKey},
		{XAI, "xai-short", ErrInvalidKey},
		{Anthropic, "", ErrInvalidKey},
		{Anthropic, goodAnthropic + " ", ErrInvalidKey},
		{Anthropic, goodAnthropic + "\n", ErrInvalidKey},
		{Anthropic, goodAnthropic + "€", ErrInvalidKey},
		{Anthropic, "sk-ant-" + strings.Repeat("a", 400), ErrInvalidKey},
		{"gemini", goodAnthropic, ErrUnknownProvider},
		{"../etc", goodAnthropic, ErrUnknownProvider},
	}
	for _, tc := range cases {
		if got := ValidateKey(tc.provider, tc.key); !errors.Is(got, tc.want) {
			t.Errorf("ValidateKey(%q, %q) = %v, want %v", tc.provider, tc.key, got, tc.want)
		}
	}
}

func TestSavedKeysAreEncryptedAndOnlyTheLastFourShow(t *testing.T) {
	svc, store, _, logs := newService(t)

	saved, err := svc.SaveKey(context.Background(), "u1", Anthropic, goodAnthropic)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Hint != "6789" || !saved.Saved {
		t.Fatalf("saved = %+v", saved)
	}
	stored := store.keys["u1"][Anthropic]
	if len(stored) == 0 || bytes.Contains(stored, []byte(goodAnthropic)) || bytes.Contains(stored, []byte("sk-ant")) {
		t.Fatal("the key must not be stored in plaintext")
	}
	if strings.Contains(logs.String(), goodAnthropic) || strings.Contains(logs.String(), "abcdefghijkl") {
		t.Fatalf("the key reached the logs: %s", logs.String())
	}

	plain, err := svc.OpenKey(context.Background(), "u1", Anthropic)
	if err != nil || plain != goodAnthropic {
		t.Fatalf("round trip = %q, %v", plain, err)
	}
}

func TestACiphertextCannotBeMovedToAnotherProviderOrPerson(t *testing.T) {
	svc, store, _, _ := newService(t)
	if _, err := svc.SaveKey(context.Background(), "u1", Anthropic, goodAnthropic); err != nil {
		t.Fatal(err)
	}
	store.keys["u1"][OpenAI] = store.keys["u1"][Anthropic]
	if _, err := svc.OpenKey(context.Background(), "u1", OpenAI); err == nil {
		t.Fatal("a key sealed for one provider opened as another")
	}
	if _, err := svc.OpenKey(context.Background(), "u2", Anthropic); !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("another person's key: %v", err)
	}
}

func TestSavingReplacesAndDeletingRemoves(t *testing.T) {
	svc, _, _, _ := newService(t)
	ctx := context.Background()
	_, _ = svc.SaveKey(ctx, "u1", Anthropic, goodAnthropic)
	replaced, err := svc.SaveKey(ctx, "u1", Anthropic, "sk-ant-api03-zyxwvutsrqponmlkjihgfedcba99")
	if err != nil || replaced.Hint != "ba99" {
		t.Fatalf("replace = %+v, %v", replaced, err)
	}
	if plain, _ := svc.OpenKey(ctx, "u1", Anthropic); !strings.HasSuffix(plain, "ba99") {
		t.Fatalf("stored key = %q", plain)
	}

	if err := svc.DeleteKey(ctx, "u1", Anthropic); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteKey(ctx, "u1", Anthropic); err != nil {
		t.Fatalf("deleting twice is not an error: %v", err)
	}
	if err := svc.DeleteKey(ctx, "u1", "bogus"); !errors.Is(err, ErrUnknownProvider) {
		t.Fatalf("unknown provider: %v", err)
	}
	if _, err := svc.OpenKey(ctx, "u1", Anthropic); !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}

func TestInvalidKeysAreRejectedBeforeAnyStorage(t *testing.T) {
	svc, store, _, _ := newService(t)
	if _, err := svc.SaveKey(context.Background(), "u1", Anthropic, "nope"); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("err = %v", err)
	}
	if len(store.keys["u1"]) != 0 {
		t.Fatal("an invalid key reached storage")
	}
}

func TestProfileListsEveryProviderInOrder(t *testing.T) {
	svc, _, _, _ := newService(t)
	_, _ = svc.SaveKey(context.Background(), "u1", XAI, "xai-abcdefghijklmnopqrstuvwxyz0123456789")

	p, err := svc.Profile(context.Background(), "u1")
	if err != nil || p.FigmaStatus != "active" {
		t.Fatalf("profile = %+v, %v", p, err)
	}
	got := []string{}
	for _, k := range p.Keys {
		got = append(got, k.Provider+":"+map[bool]string{true: "saved", false: "empty"}[k.Saved])
	}
	if strings.Join(got, ",") != "anthropic:empty,openai:empty,xai:saved" {
		t.Fatalf("keys = %v", got)
	}
}

func TestDeletingAnAccountClearsItsFilesAndOnlyIts(t *testing.T) {
	svc, store, files, _ := newService(t)
	store.imports["u1"] = []string{"imp-1", "imp-2"}
	store.imports["u2"] = []string{"imp-9"}

	if err := svc.DeleteAccount(context.Background(), "u1"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(files.cleaned, ",") != "imp-1,imp-2" {
		t.Fatalf("cleaned = %v", files.cleaned)
	}
	if !store.users["u2"] || store.users["u1"] {
		t.Fatalf("users = %v", store.users)
	}
	if err := svc.DeleteAccount(context.Background(), "u1"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}
