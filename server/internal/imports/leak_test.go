package imports

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/figma"
)

func settledGoroutines(base int) int {
	deadline := time.Now().Add(3 * time.Second)
	n := runtime.NumGoroutine()
	for n > base && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		n = runtime.NumGoroutine()
	}
	return n
}

func TestImportsLeaveNoGoroutinesBehind(t *testing.T) {
	base := runtime.NumGoroutine()

	for i := 0; i < 5; i++ {
		ok := newRig(t, okAPI())
		r := ok.settle(t, start(t, ok, ownerA, projectA, fileURL+"?node-id=1-2").ID)
		if r.Status != StatusCompleted {
			t.Fatalf("success run = %+v", r)
		}

		failing := okAPI()
		failing.nodesErr = &figma.Error{Kind: figma.KindUnavailable, Status: 500}
		bad := newRig(t, failing)
		if r := bad.settle(t, start(t, bad, ownerA, projectA, fileURL+"?node-id=1-2").ID); r.Status != StatusFailed {
			t.Fatalf("failure run = %+v", r)
		}

		blocked := okAPI()
		blocked.block = make(chan struct{})
		stuck := newRig(t, blocked)
		start(t, stuck, ownerA, projectA, fileURL+"?node-id=1-2")
		time.Sleep(20 * time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		stuck.svc.Close(ctx)
		cancel()
	}

	if n := settledGoroutines(base + 2); n > base+2 {
		buf := make([]byte, 1<<16)
		t.Fatalf("goroutines grew from %d to %d\n%s", base, n, buf[:runtime.Stack(buf, true)])
	}
}
