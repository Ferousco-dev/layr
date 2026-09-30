package imports

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/figma"
)

func TestWaitPhrasesRoundTheWayAPersonWould(t *testing.T) {
	for d, want := range map[time.Duration]string{
		20 * time.Second: "a minute", 80 * time.Second: "a minute", 10 * time.Minute: "10 minutes", 2 * time.Hour: "2 hours",
		382986 * time.Second: "4 days", 30 * time.Hour: "30 hours", 100 * time.Hour: "4 days",
	} {
		if got := WaitPhrase(d); got != want {
			t.Errorf("%s: %q, want %q", d, got, want)
		}
	}
}

func TestRateLimitedPersonIsNotSentToFigmaAgain(t *testing.T) {
	api := okAPI()
	r := newRig(t, api)
	r.svc.noteRateLimit(ownerA, 3*time.Hour)

	if _, err := r.svc.Start(context.Background(), ownerA, projectA, fileURL); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("start: %v", err)
	}
	if api.calls.Load() != 0 {
		t.Fatal("Figma was called during the wait")
	}
	if r.svc.RetryAfter(ownerB) != 0 {
		t.Fatal("the wait must belong to one person only")
	}
	if got := r.svc.RetryAfter(ownerA); got <= 2*time.Hour || got > 3*time.Hour {
		t.Fatalf("remaining wait = %s", got)
	}
}

func TestFigmaRateLimitStartsTheWait(t *testing.T) {
	api := okAPI()
	api.nodesErr = &figma.Error{Kind: figma.KindRateLimited, RetryAfter: 2 * time.Hour}
	r := newRig(t, api)

	done := r.settle(t, start(t, r, ownerA, projectA, fileURL+"?node-id=1-2").ID)

	if done.ErrorCode != "FIGMA_RATE_LIMITED" {
		t.Fatalf("import = %+v", done)
	}
	if got := r.svc.RetryAfter(ownerA); got < time.Hour+50*time.Minute || got > 2*time.Hour {
		t.Fatalf("remaining wait = %s", got)
	}
	before := api.calls.Load()
	if _, err := r.svc.Start(context.Background(), ownerA, projectA, fileURL); !errors.Is(err, ErrRateLimited) || api.calls.Load() != before {
		t.Fatalf("second attempt: %v, calls %d -> %d", err, before, api.calls.Load())
	}
}
