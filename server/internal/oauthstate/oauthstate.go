// Package oauthstate issues single-use OAuth attempts with a PKCE verifier.
package oauthstate

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"
)

const keyPrefix = "oauth:figma:state:"

var ErrInvalid = errors.New("oauth state invalid")

type KV interface {
	SetEX(ctx context.Context, key, val string, ttl time.Duration) error
	GetDel(ctx context.Context, key string) (string, bool, error)
}

type Attempt struct {
	State     string
	Challenge string
	verifier  string
}

type Store struct {
	kv  KV
	ttl time.Duration
}

func New(kv KV, ttl time.Duration) *Store { return &Store{kv: kv, ttl: ttl} }

// Begin stores the PKCE verifier under the hash of a fresh random state.
func (s *Store) Begin(ctx context.Context) (Attempt, error) {
	state, err := random()
	if err != nil {
		return Attempt{}, err
	}
	verifier, err := random()
	if err != nil {
		return Attempt{}, err
	}
	if err := s.kv.SetEX(ctx, key(state), verifier, s.ttl); err != nil {
		return Attempt{}, err
	}
	sum := sha256.Sum256([]byte(verifier))
	return Attempt{State: state, Challenge: base64.RawURLEncoding.EncodeToString(sum[:]), verifier: verifier}, nil
}

// Consume returns the verifier once; unknown, expired and replayed states are indistinguishable.
func (s *Store) Consume(ctx context.Context, state string) (string, error) {
	if !wellFormed(state) {
		return "", ErrInvalid
	}
	verifier, found, err := s.kv.GetDel(ctx, key(state))
	if err != nil {
		return "", err
	}
	if !found {
		return "", ErrInvalid
	}
	return verifier, nil
}

func wellFormed(state string) bool {
	raw, err := base64.RawURLEncoding.DecodeString(state)
	return err == nil && len(raw) == 32
}

// key hashes the state so a Redis dump does not reveal live states.
func key(state string) string {
	sum := sha256.Sum256([]byte(state))
	return keyPrefix + hex.EncodeToString(sum[:])
}

func random() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}
