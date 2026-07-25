package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// waitForListener polls addr until something accepts TCP connections or
// timeout elapses, so tests don't race the server's own startup.
func waitForListener(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("nothing listening on %s after %s", addr, timeout)
}

// doAuthedGet issues a GET with the given bearer token against a live
// server and returns the status code and response body. The *http.Response
// never escapes this function, so its Body is always closed here and
// callers can't forget to check it for nil.
func doAuthedGet(t *testing.T, url, token string) (int, []byte) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

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

// TestServeCmd_StopsOnContextCancellation mirrors daemon's #14 regression
// test: the serve command must actually exit when its context is canceled,
// not just log that it's stopping. It also exercises the real HTTP
// round-trip (auth accepted, auth rejected) against the live listener
// before shutdown, so this is more than a shutdown-timing check.
func TestServeCmd_StopsOnContextCancellation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "theia.db")
	addr := "127.0.0.1:18234"
	token := "test-token"

	serveCmd := newServeCmd()
	serveCmd.SetArgs([]string{"--db-path", dbPath, "--addr", addr, "--token", token})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- serveCmd.ExecuteContext(ctx)
	}()

	waitForListener(t, addr, 2*time.Second)

	statusOK, body := doAuthedGet(t, "http://"+addr+"/api/v1/stats", token)
	if statusOK != http.StatusOK {
		t.Fatalf("authenticated request status: got %d, want 200, body: %s", statusOK, body)
	}
	var parsed map[string]any
	if unmarshalErr := json.Unmarshal(body, &parsed); unmarshalErr != nil {
		t.Fatalf("unmarshal response: %v\nbody: %s", unmarshalErr, body)
	}
	if _, ok := parsed["series"]; !ok {
		t.Errorf("expected \"series\" key in response, got: %s", body)
	}

	statusUnauth, _ := doAuthedGet(t, "http://"+addr+"/api/v1/stats", "wrong-token")
	if statusUnauth != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request status: got %d, want 401", statusUnauth)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve command returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not stop within 5s of context cancellation")
	}
}

func TestServeCmd_MissingToken(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "theia.db")

	serveCmd := newServeCmd()
	serveCmd.SetArgs([]string{"--db-path", dbPath, "--addr", "127.0.0.1:18235"})

	err := serveCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing token, got nil")
	}
}

func TestServeCmd_NonLoopbackAddr(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "theia.db")

	serveCmd := newServeCmd()
	serveCmd.SetArgs([]string{"--db-path", dbPath, "--addr", "0.0.0.0:18236", "--token", "test-token"})

	err := serveCmd.Execute()
	if err == nil {
		t.Fatal("expected error for non-loopback addr, got nil")
	}
}

func TestServeCmd_TokenFile(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("file-token\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	got, err := resolveAPIToken("", tokenFile)
	if err != nil {
		t.Fatalf("resolveAPIToken: %v", err)
	}
	if got != "file-token" {
		t.Errorf("token: got %q, want %q (trimmed)", got, "file-token")
	}
}

func TestResolveAPIToken_Precedence(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("file-token"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	got, err := resolveAPIToken("flag-token", tokenFile)
	if err != nil {
		t.Fatalf("resolveAPIToken: %v", err)
	}
	if got != "flag-token" {
		t.Errorf("expected --token to take precedence, got %q", got)
	}
}

func TestResolveAPIToken_EnvFallback(t *testing.T) {
	t.Setenv(theiaAPITokenEnv, "env-token")

	got, err := resolveAPIToken("", "")
	if err != nil {
		t.Fatalf("resolveAPIToken: %v", err)
	}
	if got != "env-token" {
		t.Errorf("expected env var fallback, got %q", got)
	}
}

func TestResolveAPIToken_NoneConfigured(t *testing.T) {
	if _, err := resolveAPIToken("", ""); err == nil {
		t.Error("expected error when no token source is configured, got nil")
	}
}
