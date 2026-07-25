package apiserver_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/Elysium-Labs-EU/theia/internal/apiserver"
)

func TestRun_ShutsDownCleanlyOnContextCancel(t *testing.T) {
	db := setupTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- apiserver.Run(ctx, db, apiserver.Config{Addr: "127.0.0.1:0", Token: testToken})
	}()

	// Give the listener a moment to come up before triggering shutdown.
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

	// Occupy a port first so the second listener on the same address fails.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve a port: %v", err)
	}
	addr := listener.Addr().String()
	defer func() { _ = listener.Close() }()

	err = apiserver.Run(context.Background(), db, apiserver.Config{Addr: addr, Token: testToken})
	if err == nil {
		t.Fatal("Run() with an already-bound address = nil error, want non-nil")
	}
}

func TestValidateLoopbackAddr(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		wantErr bool
	}{
		{"loopback ip", "127.0.0.1:8081", false},
		{"localhost", "localhost:8081", false},
		{"all interfaces", "0.0.0.0:8081", true},
		{"external host", "example.com:8081", true},
		{"missing port", "127.0.0.1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := apiserver.ValidateLoopbackAddr(tt.addr)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateLoopbackAddr(%q) error = %v, wantErr %v", tt.addr, err, tt.wantErr)
			}
		})
	}
}
