package oauthstate

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeKV struct {
	mu   sync.Mutex
	now  time.Time
	data map[string]entry
}

type entry struct {
	val     string
	expires time.Time
}

func newFake() *fakeKV { return &fakeKV{now: time.Unix(1000, 0), data: map[string]entry{}} }

func (f *fakeKV) SetEX(_ context.Context, k, v string, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[k] = entry{v, f.now.Add(ttl)}
	return nil
}

func (f *fakeKV) GetDel(_ context.Context, k string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.data[k]
	delete(f.data, k)
	if !ok || !f.now.Before(e.expires) {
		return "", false, nil
	}
	return e.val, true, nil
}

func TestBeginThenConsumeReturnsVerifier(t *testing.T) {
	s := New(newFake(), time.Minute)

	a, err := s.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.Consume(context.Background(), a.State)

	if err != nil || v != a.verifier || a.Challenge == "" {
		t.Fatalf("v=%q err=%v", v, err)
	}
}

func TestConsumeIsSingleUse(t *testing.T) {
	s := New(newFake(), time.Minute)
	a, _ := s.Begin(context.Background())

	_, _ = s.Consume(context.Background(), a.State)
	_, err := s.Consume(context.Background(), a.State)

	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("replay err = %v", err)
	}
}

func TestConsumeRejectsExpiredUnknownAndMalformed(t *testing.T) {
	kv := newFake()
	s := New(kv, time.Minute)
	a, _ := s.Begin(context.Background())
	kv.now = kv.now.Add(2 * time.Minute)
	unknown, _ := New(newFake(), time.Minute).Begin(context.Background())

	for name, state := range map[string]string{"expired": a.State, "unknown": unknown.State, "malformed": "!!", "empty": ""} {
		if _, err := s.Consume(context.Background(), state); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s: err = %v", name, err)
		}
	}
}

func TestStatesAreUniqueAndHashedInStorage(t *testing.T) {
	kv := newFake()
	s := New(kv, time.Minute)

	a, _ := s.Begin(context.Background())
	b, _ := s.Begin(context.Background())

	if a.State == b.State {
		t.Fatal("states must differ")
	}
	for k := range kv.data {
		if strings.Contains(k, a.State) {
			t.Fatal("raw state present in storage key")
		}
	}
}
