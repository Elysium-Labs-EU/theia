package promsink_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/Elysium-Labs-EU/theia/internal/promsink"
)

func TestRun_ShutsDownCleanlyOnContextCancel(t *testing.T) {
	db := setupTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- promsink.Run(ctx, db, promsink.Config{Addr: "127.0.0.1:0", Top: 20})
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run() on context cancel = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}

func TestRun_ReturnsErrorWhenListenFails(t *testing.T) {
	db := setupTestDB(t)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve a port: %v", err)
	}
	addr := listener.Addr().String()
	defer func() { _ = listener.Close() }()

	err = promsink.Run(context.Background(), db, promsink.Config{Addr: addr, Top: 20})
	if err == nil {
		t.Fatal("Run() with an already-bound address = nil error, want non-nil")
	}
}

func TestNewServer_ServesMetricsRoute(t *testing.T) {
	db := setupTestDB(t)
	srv := promsink.NewServer(db, promsink.Config{Top: 20})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+ln.Addr().String()+"/metrics", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp == nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if len(body) == 0 {
		t.Error("expected non-empty metrics body")
	}
}
