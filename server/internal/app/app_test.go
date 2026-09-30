package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/observability"
)

type fakeServer struct {
	shutdownStarted chan struct{}
	releaseShutdown chan struct{}
	closed          chan struct{}
	once            sync.Once
	order           *[]string
}

func (s *fakeServer) Serve(net.Listener) error {
	<-s.closed
	return nil
}

func (s *fakeServer) Shutdown(context.Context) error {
	*s.order = append(*s.order, "http")
	close(s.shutdownStarted)
	<-s.releaseShutdown
	_ = s.Close()
	return nil
}

func (s *fakeServer) Close() error {
	s.once.Do(func() { close(s.closed) })
	return nil
}

type fakeRedis struct{ order *[]string }

func (r *fakeRedis) Close() error {
	*r.order = append(*r.order, "redis")
	return nil
}

type fakePostgres struct{ order *[]string }

func (p *fakePostgres) Close() { *p.order = append(*p.order, "postgres") }

func TestRunLifecycleClosesResourcesInOrder(t *testing.T) {
	order := []string{}
	server := &fakeServer{
		shutdownStarted: make(chan struct{}),
		releaseShutdown: make(chan struct{}),
		closed:          make(chan struct{}),
		order:           &order,
	}
	redis := &fakeRedis{order: &order}
	postgres := &fakePostgres{order: &order}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	close(server.releaseShutdown)

	err := runLifecycle(
		ctx,
		make(chan struct{}),
		server,
		nil,
		redis,
		postgres,
		observability.New(&bytes.Buffer{}, slog.LevelInfo),
		time.Second,
		func(context.Context) { order = append(order, "imports") },
	)

	if err != nil {
		t.Fatal(err)
	}
	want := []string{"http", "imports", "redis", "postgres"}
	if len(order) != len(want) {
		t.Fatalf("close order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("close order = %v, want %v", order, want)
		}
	}
}

func TestRunLifecycleSecondSignalForcesClose(t *testing.T) {
	order := []string{}
	server := &fakeServer{
		shutdownStarted: make(chan struct{}),
		releaseShutdown: make(chan struct{}),
		closed:          make(chan struct{}),
		order:           &order,
	}
	ctx, cancel := context.WithCancel(context.Background())
	force := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		done <- runLifecycle(
			ctx,
			force,
			server,
			nil,
			&fakeRedis{order: &order},
			&fakePostgres{order: &order},
			observability.New(&bytes.Buffer{}, slog.LevelInfo),
			time.Second,
			nil,
		)
	}()
	cancel()
	<-server.shutdownStarted
	close(force)

	err := <-done
	if err == nil || err.Error() != "shutdown forced" {
		t.Fatalf("got %v, want forced-shutdown error", err)
	}
	select {
	case <-server.closed:
	default:
		t.Fatal("server close was not forced")
	}
}

type failingServer struct{ fakeServer }

func (s *failingServer) Serve(net.Listener) error { return errors.New("boom") }

func TestRunLifecycleServeFailureClosesResources(t *testing.T) {
	order := []string{}
	server := &failingServer{}
	redis := &fakeRedis{order: &order}
	postgres := &fakePostgres{order: &order}

	err := runLifecycle(
		context.Background(),
		make(chan struct{}),
		server,
		nil,
		redis,
		postgres,
		observability.New(&bytes.Buffer{}, slog.LevelInfo),
		time.Second,
		nil,
	)

	if err == nil {
		t.Fatal("expected error")
	}
	want := []string{"redis", "postgres"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("close order = %v, want %v", order, want)
	}
}
