// Package redis owns the ephemeral Redis client (DES-022).
package redis

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/ferousco-dev/layr/server/internal/config"
	goredis "github.com/redis/go-redis/v9"
)

var ErrUnavailable = errors.New("redis unavailable")

type Client struct {
	c    *goredis.Client
	once sync.Once
	err  error
}

func Open(ctx context.Context, c config.Redis) (*Client, error) {
	o, e := goredis.ParseURL(string(c.URL))
	if e != nil {
		return nil, ErrUnavailable
	}
	o.PoolSize = c.PoolSize
	x := &Client{c: goredis.NewClient(o)}
	if e = x.c.Ping(ctx).Err(); e != nil {
		_ = x.Close()
		return nil, ErrUnavailable
	}
	return x, nil
}

func (c *Client) Ping(ctx context.Context) error {
	if e := c.c.Ping(ctx).Err(); e != nil {
		return ErrUnavailable
	}
	return nil
}

func (c *Client) Close() error {
	c.once.Do(func() {
		c.err = c.c.Close()
	})
	return c.err
}

// SetEX stores a value that expires after ttl.
func (c *Client) SetEX(ctx context.Context, key, val string, ttl time.Duration) error {
	if err := c.c.Set(ctx, key, val, ttl).Err(); err != nil {
		return ErrUnavailable
	}
	return nil
}

// GetDel reads and deletes atomically so a value can be consumed once.
func (c *Client) GetDel(ctx context.Context, key string) (string, bool, error) {
	val, err := c.c.GetDel(ctx, key).Result()
	if errors.Is(err, goredis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, ErrUnavailable
	}
	return val, true, nil
}

// Acquire takes a lock with a random owner token and a bounded ttl.
func (c *Client) Acquire(ctx context.Context, key string, ttl time.Duration) (string, bool, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", false, err
	}
	owner := hex.EncodeToString(raw[:])
	ok, err := c.c.SetNX(ctx, key, owner, ttl).Result()
	if err != nil {
		return "", false, ErrUnavailable
	}
	return owner, ok, nil
}

var releaseScript = goredis.NewScript(`if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) end return 0`)

// Release deletes the lock only while the caller still owns it.
func (c *Client) Release(ctx context.Context, key, owner string) error {
	if err := releaseScript.Run(ctx, c.c, []string{key}, owner).Err(); err != nil {
		return ErrUnavailable
	}
	return nil
}

var countScript = goredis.NewScript(`local n = redis.call("incr", KEYS[1]) if n == 1 then redis.call("pexpire", KEYS[1], ARGV[1]) end return n`)

// Count increments a fixed-window counter and returns the new total.
func (c *Client) Count(ctx context.Context, key string, window time.Duration) (int64, error) {
	n, err := countScript.Run(ctx, c.c, []string{key}, window.Milliseconds()).Int64()
	if err != nil {
		return 0, ErrUnavailable
	}
	return n, nil
}
