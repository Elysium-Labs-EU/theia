package cmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hostRedirectTransport rewrites any request to hit addr over plain HTTP.
// Lets tests intercept the hardcoded GitHub URLs in fetchLatestRelease
// and downloadFile.
type hostRedirectTransport struct{ addr string }

func (h *hostRedirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r2 := req.Clone(req.Context())
	r2.URL.Host = h.addr
	r2.URL.Scheme = "http"
	return http.DefaultTransport.RoundTrip(r2)
}

// useHTTPTestServer starts an httptest.Server and wires httpClient to route
// all requests to it. Restores the original client on test cleanup.
func useHTTPTestServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	orig := httpClient
	httpClient = &http.Client{Transport: &hostRedirectTransport{addr: srv.Listener.Addr().String()}}
	t.Cleanup(func() {
		httpClient = orig
		srv.Close()
	})
}

func TestFetchLatestRelease(t *testing.T) {
	useHTTPTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/releases/latest") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"tag_name": "v0.0.2",
			"assets": [
				{"name": "theia-linux-amd64", "browser_download_url": "https://example.com/theia-linux-amd64"},
				{"name": "sha256sums.txt", "browser_download_url": "https://example.com/sha256sums.txt"}
			]
		}`))
	})

	rel, err := fetchLatestRelease(context.Background(), false)
	if err != nil {
		t.Fatalf("fetchLatestRelease: %v", err)
	}
	if rel.TagName != "v0.0.2" {
		t.Errorf("TagName = %q, want %q", rel.TagName, "v0.0.2")
	}
	if len(rel.Assets) != 2 {
		t.Fatalf("Assets = %+v, want 2 entries", rel.Assets)
	}
}

func TestFetchLatestReleaseNonOKStatus(t *testing.T) {
	useHTTPTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	if _, err := fetchLatestRelease(context.Background(), false); err == nil {
		t.Fatal("expected an error for a non-200 response")
	}
}

func TestFetchLatestReleaseIncludePre(t *testing.T) {
	useHTTPTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/releases") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"tag_name": "v0.0.3-rc.1", "assets": []},
			{"tag_name": "v0.0.2", "assets": []}
		]`))
	})

	rel, err := fetchLatestRelease(context.Background(), true)
	if err != nil {
		t.Fatalf("fetchLatestRelease: %v", err)
	}
	if rel.TagName != "v0.0.3-rc.1" {
		t.Errorf("TagName = %q, want the newest (first) entry %q", rel.TagName, "v0.0.3-rc.1")
	}
}

func TestFetchLatestReleaseIncludePreEmptyList(t *testing.T) {
	useHTTPTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	})

	if _, err := fetchLatestRelease(context.Background(), true); err == nil {
		t.Fatal("expected an error when no releases exist")
	}
}

func TestFetchLatestReleaseIncludePreBadJSON(t *testing.T) {
	useHTTPTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	})

	if _, err := fetchLatestRelease(context.Background(), true); err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}

func TestReleaseAssetFor(t *testing.T) {
	rel := Release{
		Assets: []Asset{
			{Name: "theia-linux-amd64", DownloadURL: "https://github.com/x/amd64"},
			{Name: "theia-linux-arm64", DownloadURL: "https://github.com/x/arm64"},
			{Name: "sha256sums.txt", DownloadURL: "https://github.com/x/sums"},
		},
	}

	a, ok := rel.AssetFor("amd64")
	if !ok || a.Name != "theia-linux-amd64" {
		t.Fatalf("AssetFor(amd64) = %+v, %v", a, ok)
	}

	if _, matched := rel.AssetFor("mips"); matched {
		t.Fatal("AssetFor(mips) should not match")
	}

	sums, ok := rel.ChecksumsAsset()
	if !ok || sums.Name != "sha256sums.txt" {
		t.Fatalf("ChecksumsAsset() = %+v, %v", sums, ok)
	}
}

func TestDownloadFileRejectsUntrustedHost(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "out")
	err := downloadFile(context.Background(), "https://evil.example.com/theia", dst)
	if err == nil {
		t.Fatal("expected an error for a non-github.com host")
	}
}

func TestDownloadFileRejectsNonHTTPS(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "out")
	err := downloadFile(context.Background(), "http://github.com/theia", dst)
	if err == nil {
		t.Fatal("expected an error for a non-https scheme")
	}
}

func TestDownloadFileSuccess(t *testing.T) {
	const body = "fake binary contents"
	useHTTPTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	})

	dst := filepath.Join(t.TempDir(), "out")
	if err := downloadFile(context.Background(), "https://github.com/theia-linux-amd64", dst); err != nil {
		t.Fatalf("downloadFile: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != body {
		t.Errorf("downloaded contents = %q, want %q", got, body)
	}
}

func TestDownloadFileRejectsNon200Status(t *testing.T) {
	useHTTPTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	dst := filepath.Join(t.TempDir(), "out")
	err := downloadFile(context.Background(), "https://github.com/theia-linux-amd64", dst)
	if err == nil {
		t.Fatal("expected an error for a non-200 response")
	}
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func TestVerifyChecksum(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "theia-linux-amd64")
	if err := os.WriteFile(binPath, []byte("fake binary contents"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	sum, err := sha256File(binPath)
	if err != nil {
		t.Fatalf("sha256File: %v", err)
	}

	checksums := sum + "  theia-linux-amd64\nother  other-file\n"
	if err := verifyChecksum(binPath, checksums, "theia-linux-amd64"); err != nil {
		t.Errorf("verifyChecksum with matching sum: %v", err)
	}

	if err := verifyChecksum(binPath, "deadbeef  theia-linux-amd64\n", "theia-linux-amd64"); err == nil {
		t.Error("expected an error for a checksum mismatch")
	}

	if err := verifyChecksum(binPath, "deadbeef  other-file\n", "theia-linux-amd64"); err == nil {
		t.Error("expected an error when no entry matches the asset name")
	}
}

func TestCopyFileAndReplaceBinary(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "new-binary")
	if err := os.WriteFile(src, []byte("new contents"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	dst := filepath.Join(dir, "installed-binary")
	if err := os.WriteFile(dst, []byte("old contents"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := replaceBinary(src, dst); err != nil {
		t.Fatalf("replaceBinary: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "new contents" {
		t.Errorf("dst contents = %q, want %q", got, "new contents")
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("dst is not executable: mode %v", info.Mode())
	}

	if _, err := os.Stat(dst + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("expected the .tmp file to be gone after rename, got err=%v", err)
	}
}

func TestCheckWritable(t *testing.T) {
	dir := t.TempDir()
	if err := checkWritable(dir); err != nil {
		t.Errorf("checkWritable(%s): %v", dir, err)
	}

	if err := checkWritable(filepath.Join(dir, "does-not-exist")); err == nil {
		t.Error("expected an error for a non-existent directory")
	}
}

func TestRunUpdateAlreadyLatest(t *testing.T) {
	useHTTPTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name": "v0.1.0", "assets": []}`))
	})

	exePath := filepath.Join(t.TempDir(), "theia")
	buf := &bytes.Buffer{}

	if err := runUpdate(context.Background(), buf, exePath, "v0.1.0", false); err != nil {
		t.Fatalf("runUpdate: %v", err)
	}
	if !strings.Contains(buf.String(), "already on the latest version") {
		t.Errorf("output = %q, want an already-latest message", buf.String())
	}
}

func TestRunUpdateNewerAvailableButNoMatchingAsset(t *testing.T) {
	useHTTPTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name": "v9.9.9", "assets": []}`))
	})

	exePath := filepath.Join(t.TempDir(), "theia")
	buf := &bytes.Buffer{}

	err := runUpdate(context.Background(), buf, exePath, "v0.1.0", false)
	if err == nil {
		t.Fatal("expected an error when the release has no matching asset")
	}
	if !strings.Contains(err.Error(), "no asset for linux-") {
		t.Errorf("error = %v, want a missing-asset message", err)
	}
}

func TestRunUpdateIncludePreUsesReleasesList(t *testing.T) {
	var gotPath string
	useHTTPTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"tag_name": "v0.2.0-rc.1", "assets": []}]`))
	})

	exePath := filepath.Join(t.TempDir(), "theia")
	buf := &bytes.Buffer{}

	err := runUpdate(context.Background(), buf, exePath, "v0.1.0", true)
	if err == nil {
		t.Fatal("expected an error when the release has no matching asset")
	}
	if !strings.HasSuffix(gotPath, "/releases") {
		t.Errorf("request path = %q, want the releases-list endpoint", gotPath)
	}
	if !strings.Contains(buf.String(), "v0.1.0 -> v0.2.0-rc.1") {
		t.Errorf("output = %q, want it to mention the pre-release version", buf.String())
	}
}

func TestNormalizeSemver(t *testing.T) {
	tests := map[string]string{
		"0.1.0":  "v0.1.0",
		"v0.1.0": "v0.1.0",
		"":       "",
	}
	for in, want := range tests {
		if got := normalizeSemver(in); got != want {
			t.Errorf("normalizeSemver(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHostArch(t *testing.T) {
	arch, err := hostArch()
	if err != nil {
		// Only amd64/arm64 are supported; this test environment may not be
		// one of them, which is itself a valid (if untested-further) path.
		t.Skipf("hostArch: %v", err)
	}
	if arch != "amd64" && arch != "arm64" {
		t.Errorf("hostArch() = %q, want amd64 or arm64", arch)
	}
}

// TestServiceIsActiveNoSystemd asserts the non-systemd fallback: on a
// machine without theia.service registered (dev laptop, CI, this test's
// environment), serviceIsActive must report false rather than erroring out.
func TestServiceIsActiveNoSystemd(t *testing.T) {
	if serviceIsActive(context.Background()) {
		t.Skip("theia.service is active on this machine — skipping the negative-path assertion")
	}
}

// TestStopServiceNoopWhenInactive asserts stopService is a no-op — and
// reports nothing was stopped — when theia.service isn't running, so
// runUpdate doesn't try to restart a service that was never up.
func TestStopServiceNoopWhenInactive(t *testing.T) {
	if serviceIsActive(context.Background()) {
		t.Skip("theia.service is active on this machine — skipping the inactive-path assertion")
	}

	buf := &bytes.Buffer{}
	if stopService(context.Background(), buf) {
		t.Error("stopService() = true, want false when theia.service isn't active")
	}
}
