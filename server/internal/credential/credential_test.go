package credential

import (
	"bytes"
	"errors"
	"testing"
)

func testCipher(t *testing.T, fill byte) *Cipher {
	t.Helper()
	c, err := New(bytes.Repeat([]byte{fill}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSealOpenRoundTrip(t *testing.T) {
	c := testCipher(t, 1)

	sealed, err := c.Seal([]byte("fake-access-token"), "access")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := c.Open(sealed, "access")

	if err != nil || string(plain) != "fake-access-token" {
		t.Fatalf("plain = %q, err = %v", plain, err)
	}
	if bytes.Contains(sealed, []byte("fake-access-token")) {
		t.Fatal("ciphertext contains plaintext")
	}
}

func TestSealUsesFreshNonce(t *testing.T) {
	c := testCipher(t, 1)

	a, _ := c.Seal([]byte("same"), "access")
	b, _ := c.Seal([]byte("same"), "access")

	if bytes.Equal(a, b) {
		t.Fatal("identical ciphertexts imply nonce reuse")
	}
}

func TestOpenRejectsTamperingWrongPurposeAndWrongKey(t *testing.T) {
	c := testCipher(t, 1)
	sealed, _ := c.Seal([]byte("secret"), "access")

	tampered := append([]byte(nil), sealed...)
	tampered[len(tampered)-1] ^= 0xff
	if _, err := c.Open(tampered, "access"); !errors.Is(err, ErrMalformed) {
		t.Fatalf("tampered: %v", err)
	}
	if _, err := c.Open(sealed, "refresh"); !errors.Is(err, ErrMalformed) {
		t.Fatalf("wrong purpose: %v", err)
	}
	if _, err := testCipher(t, 2).Open(sealed, "access"); !errors.Is(err, ErrMalformed) {
		t.Fatalf("wrong key: %v", err)
	}
}

func TestOpenRejectsMalformedInput(t *testing.T) {
	c := testCipher(t, 1)

	if _, err := c.Open(nil, "access"); !errors.Is(err, ErrMalformed) {
		t.Fatalf("empty: %v", err)
	}
	if _, err := c.Open(bytes.Repeat([]byte{9}, 64), "access"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("version: %v", err)
	}
}

func TestNewRejectsBadKeyLength(t *testing.T) {
	if _, err := New([]byte("short")); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("err = %v", err)
	}
}
