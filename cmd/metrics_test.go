package cmd

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func doGet(t *testing.T, url string) (int, []byte) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp == nil {
		t.Fatalf("request: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, body
}

func TestServeMetricsCmd_StopsOnContextCancellation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "theia.db")
	addr := "127.0.0.1:18237"

	serveMetricsCmd := newServeMetricsCmd()
	serveMetricsCmd.SetArgs([]string{"--db-path", dbPath, "--addr", addr})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- serveMetricsCmd.ExecuteContext(ctx)
	}()

	waitForListener(t, addr, 2*time.Second)

	status, body := doGet(t, "http://"+addr+"/metrics")
	if status != http.StatusOK {
		t.Fatalf("GET /metrics status: got %d, want 200, body: %s", status, body)
	}
	if !strings.Contains(string(body), "theia_pageviews_total") {
		t.Errorf("expected theia_pageviews_total in response, got: %s", body)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve-metrics command returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve-metrics did not stop within 5s of context cancellation")
	}
}

func TestServeMetricsCmd_NonLoopbackAddr(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "theia.db")

	serveMetricsCmd := newServeMetricsCmd()
	serveMetricsCmd.SetArgs([]string{"--db-path", dbPath, "--addr", "0.0.0.0:18238"})

	err := serveMetricsCmd.Execute()
	if err == nil {
		t.Fatal("expected error for non-loopback addr, got nil")
	}
}

func TestServeMetricsCmd_DBOpenFailure(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "no-such-dir", "theia.db")

	serveMetricsCmd := newServeMetricsCmd()
	serveMetricsCmd.SetArgs([]string{"--db-path", dbPath, "--addr", "127.0.0.1:18240"})

	err := serveMetricsCmd.Execute()
	if err == nil {
		t.Fatal("expected error when db-path's parent directory does not exist, got nil")
	}
}

func TestServeMetricsCmd_InvalidTop(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "theia.db")

	for _, top := range []string{"0", "-5"} {
		serveMetricsCmd := newServeMetricsCmd()
		serveMetricsCmd.SetArgs([]string{"--db-path", dbPath, "--addr", "127.0.0.1:18239", "--top", top})

		err := serveMetricsCmd.Execute()
		if err == nil {
			t.Fatalf("expected error for --top %s, got nil", top)
		}
	}
}
