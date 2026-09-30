package redis

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/config"
)

func liveClient(t *testing.T) *Client {
	t.Helper()
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL not set")
	}
	c, err := Open(context.Background(), config.Redis{URL: config.SecretURL(url), PoolSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestLiveGetDelConsumesOnce(t *testing.T) {
	c, ctx := liveClient(t), context.Background()
	_ = c.SetEX(ctx, "test:getdel", "v", time.Minute)

	first, ok1, _ := c.GetDel(ctx, "test:getdel")
	_, ok2, _ := c.GetDel(ctx, "test:getdel")

	if !ok1 || first != "v" || ok2 {
		t.Fatalf("first=%q ok1=%v ok2=%v", first, ok1, ok2)
	}
}

func TestLiveLockExcludesAndReleasesByOwner(t *testing.T) {
	c, ctx := liveClient(t), context.Background()
	key := "test:lock:" + strconv.FormatInt(time.Now().UnixNano(), 36)

	owner, ok, _ := c.Acquire(ctx, key, time.Minute)
	_, second, _ := c.Acquire(ctx, key, time.Minute)
	_ = c.Release(ctx, key, "not-the-owner")
	_, stillHeld, _ := c.Acquire(ctx, key, time.Minute)
	_ = c.Release(ctx, key, owner)
	again, afterRelease, _ := c.Acquire(ctx, key, time.Minute)
	_ = c.Release(ctx, key, again)

	if !ok || second || stillHeld || !afterRelease {
		t.Fatalf("ok=%v second=%v stillHeld=%v afterRelease=%v", ok, second, stillHeld, afterRelease)
	}
}

func TestLiveCountIncrementsWithinWindow(t *testing.T) {
	c, ctx := liveClient(t), context.Background()
	key := "test:count:" + strconv.FormatInt(time.Now().UnixNano(), 36)

	a, _ := c.Count(ctx, key, time.Minute)
	b, _ := c.Count(ctx, key, time.Minute)

	if a != 1 || b != 2 {
		t.Fatalf("a=%d b=%d", a, b)
	}
}
