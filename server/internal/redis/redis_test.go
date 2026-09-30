package redis

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/config"
)

func TestOpenRejectsMalformedURL(t *testing.T) {
	_, err := Open(context.Background(), config.Redis{URL: "://bad", PoolSize: 2})

	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestOpenReportsUnreachableServerWithoutLeakingURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := Open(ctx, config.Redis{URL: "redis://:secret@127.0.0.1:1/0", PoolSize: 2})

	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
	if err.Error() != ErrUnavailable.Error() {
		t.Fatalf("error text carries detail: %q", err.Error())
	}
}
