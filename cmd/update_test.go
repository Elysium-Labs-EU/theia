package cmd

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
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

// TestFetchLatestReleaseIncludePreOutOfOrderList asserts includePre picks
// the highest semver tag rather than trusting GitHub's list order — GitHub
// has been observed to return a freshly created release out of position
// (see theia issue #36 / argus#74).
func TestFetchLatestReleaseIncludePreOutOfOrderList(t *testing.T) {
	useHTTPTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"tag_name": "v0.1.0", "prerelease": false, "assets": []},
			{"tag_name": "v0.3.0", "prerelease": false, "assets": []},
			{"tag_name": "v0.2.0", "prerelease": false, "assets": []}
		]`))
	})

	rel, err := fetchLatestRelease(context.Background(), true)
	if err != nil {
		t.Fatalf("fetchLatestRelease: %v", err)
	}
	if rel.TagName != "v0.3.0" {
		t.Errorf("TagName = %q, want the highest-semver entry %q even though it's not first in the list", rel.TagName, "v0.3.0")
	}
}

// TestFetchLatestReleaseIncludePreAllPrerelease asserts includePre falls
// back to the highest-semver prerelease when no stable release exists.
func TestFetchLatestReleaseIncludePreAllPrerelease(t *testing.T) {
	useHTTPTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"tag_name": "v0.2.0-rc.1", "prerelease": true, "assets": []},
			{"tag_name": "v0.3.0-rc.1", "prerelease": true, "assets": []}
		]`))
	})

	rel, err := fetchLatestRelease(context.Background(), true)
	if err != nil {
		t.Fatalf("fetchLatestRelease: %v", err)
	}
	if rel.TagName != "v0.3.0-rc.1" {
		t.Errorf("TagName = %q, want the highest-semver prerelease %q", rel.TagName, "v0.3.0-rc.1")
	}
}

// TestFetchLatestReleaseIncludePrePrefersStableOverNewerPrerelease asserts
// a stable release always wins over a prerelease, even one with a higher
// tag, since pickBestRelease only falls back to prereleases when no stable
// release exists.
func TestFetchLatestReleaseIncludePrePrefersStableOverNewerPrerelease(t *testing.T) {
	useHTTPTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"tag_name": "v0.4.0-rc.1", "prerelease": true, "assets": []},
			{"tag_name": "v0.3.0", "prerelease": false, "assets": []}
		]`))
	})

	rel, err := fetchLatestRelease(context.Background(), true)
	if err != nil {
		t.Fatalf("fetchLatestRelease: %v", err)
	}
	if rel.TagName != "v0.3.0" {
		t.Errorf("TagName = %q, want the stable release %q even though a newer prerelease exists", rel.TagName, "v0.3.0")
	}
}

// TestFetchLatestReleasePlainPathFallsBackOn404 asserts the plain
// (non-includePre) path falls back to the full /releases list when
// /releases/latest 404s — the case where every published release is a
// prerelease, which GitHub's "latest" endpoint always excludes.
func TestFetchLatestReleasePlainPathFallsBackOn404(t *testing.T) {
	useHTTPTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			w.WriteHeader(http.StatusNotFound)
		case strings.HasSuffix(r.URL.Path, "/releases"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"tag_name": "v0.2.0-rc.1", "prerelease": true, "assets": []},
				{"tag_name": "v0.3.0-rc.2", "prerelease": true, "assets": []}
			]`))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	})

	rel, err := fetchLatestRelease(context.Background(), false)
	if err != nil {
		t.Fatalf("fetchLatestRelease: %v", err)
	}
	if rel.TagName != "v0.3.0-rc.2" {
		t.Errorf("TagName = %q, want the highest-semver prerelease %q from the fallback list", rel.TagName, "v0.3.0-rc.2")
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

func TestReleaseSignatureAsset(t *testing.T) {
	rel := Release{
		Assets: []Asset{
			{Name: "theia-linux-amd64", DownloadURL: "https://github.com/x/amd64"},
			{Name: "sha256sums.txt.sig", DownloadURL: "https://github.com/x/sig"},
		},
	}
	sig, ok := rel.SignatureAsset()
	if !ok || sig.Name != "sha256sums.txt.sig" {
		t.Fatalf("SignatureAsset() = %+v, %v", sig, ok)
	}

	noSig := Release{Assets: []Asset{{Name: "theia-linux-amd64"}}}
	if _, ok := noSig.SignatureAsset(); ok {
		t.Error("SignatureAsset() should not match when no sig asset is present")
	}
}

func TestParseReleaseSigningPublicKey(t *testing.T) {
	pub, err := parseReleaseSigningPublicKey()
	if err != nil {
		t.Fatalf("parseReleaseSigningPublicKey: %v", err)
	}
	if pub.Curve != elliptic.P256() {
		t.Errorf("public key curve = %v, want P-256", pub.Curve)
	}
}

// generateTestSigningKey returns a throwaway ECDSA P-256 key for signing
// test fixtures — never the embedded production key, which has no private
// half checked into this repo.
func generateTestSigningKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating test signing key: %v", err)
	}
	return key
}

func TestVerifySignature(t *testing.T) {
	key := generateTestSigningKey(t)
	data := []byte("theia release checksums fixture")

	digest := sha256.Sum256(data)
	sig, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatalf("signing fixture: %v", err)
	}

	if err := verifySignature(&key.PublicKey, data, sig); err != nil {
		t.Errorf("verifySignature with a valid signature: %v", err)
	}

	if err := verifySignature(&key.PublicKey, []byte("tampered data"), sig); err == nil {
		t.Error("expected an error when the signed data doesn't match")
	}

	otherKey := generateTestSigningKey(t)
	if err := verifySignature(&otherKey.PublicKey, data, sig); err == nil {
		t.Error("expected an error when the signature was made by a different key")
	}

	if err := verifySignature(&key.PublicKey, data, []byte("not a signature")); err == nil {
		t.Error("expected an error for a malformed signature")
	}
}

func TestVerifyChecksumsSignatureRejectsForgedSignature(t *testing.T) {
	// verifyChecksumsSignature always checks against the embedded production
	// public key, so a signature not produced by its (deliberately absent)
	// private key must be rejected regardless of content.
	if err := verifyChecksumsSignature([]byte("sha256sums.txt contents"), []byte("forged signature bytes")); err == nil {
		t.Error("expected an error for a signature not made by the production key")
	}
}

func TestVerifyReleaseSignatureMissingAssetWarnsAndContinues(t *testing.T) {
	useHTTPTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request to %s — no signature asset should mean no download", r.URL.Path)
	})

	rel := Release{TagName: "v1.0.0"} // no sha256sums.txt.sig asset
	buf := &bytes.Buffer{}

	if err := verifyReleaseSignature(context.Background(), buf, rel, []byte("checksums"), t.TempDir()); err != nil {
		t.Fatalf("verifyReleaseSignature: %v", err)
	}
	if !strings.Contains(buf.String(), "has no signature") {
		t.Errorf("output = %q, want a no-signature warning", buf.String())
	}
}

func TestVerifyReleaseSignaturePresentButInvalidFails(t *testing.T) {
	useHTTPTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not a real signature"))
	})

	rel := Release{
		TagName: "v1.0.0",
		Assets:  []Asset{{Name: "sha256sums.txt.sig", DownloadURL: "https://github.com/x/sig"}},
	}
	buf := &bytes.Buffer{}

	err := verifyReleaseSignature(context.Background(), buf, rel, []byte("checksums"), t.TempDir())
	if err == nil {
		t.Fatal("expected an error for an invalid signature")
	}
	if !strings.Contains(err.Error(), "signature verification failed") {
		t.Errorf("error = %v, want a signature-verification-failed message", err)
	}
}

// TestRunUpdateNoSignatureAssetStillUpdates confirms the current soft-fail
// policy (requireReleaseSignature == false): a release published before
// signing existed has no sha256sums.txt.sig, and the update proceeds with a
// warning rather than being refused.
func TestRunUpdateNoSignatureAssetStillUpdates(t *testing.T) {
	arch := runtime.GOARCH
	if arch != "amd64" && arch != "arm64" {
		t.Skipf("unsupported test arch %q", arch)
	}
	t.Setenv("HOME", t.TempDir())

	assetName := "theia-linux-" + arch
	binContent := []byte("signed-less binary\n")
	sum := sha256.Sum256(binContent)
	checksums := hex.EncodeToString(sum[:]) + "  " + assetName + "\n"

	useHTTPTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"tag_name":"v9.9.9","assets":[`+
				`{"name":%q,"browser_download_url":"https://github.com/dl/%s"},`+
				`{"name":"sha256sums.txt","browser_download_url":"https://github.com/dl/sha256sums.txt"}]}`,
				assetName, assetName)
		case strings.HasSuffix(r.URL.Path, "sha256sums.txt"):
			_, _ = w.Write([]byte(checksums))
		case strings.HasSuffix(r.URL.Path, assetName):
			_, _ = w.Write(binContent)
		default:
			t.Errorf("unexpected request path %s", r.URL.Path)
		}
	})

	exePath := filepath.Join(t.TempDir(), "theia")
	if err := os.WriteFile(exePath, []byte("old theia"), 0o755); err != nil {
		t.Fatalf("seeding exe: %v", err)
	}

	buf := &bytes.Buffer{}
	if err := runUpdate(context.Background(), buf, exePath, "v0.1.0", false); err != nil {
		t.Fatalf("runUpdate: %v", err)
	}
	if !strings.Contains(buf.String(), "has no signature") {
		t.Errorf("output = %q, want a no-signature warning", buf.String())
	}
}

// TestRunUpdateInvalidSignatureBlocksInstall confirms a release that
// published a signature but fails to verify is always refused, even under
// the soft-fail policy — that policy only excuses a signature's absence,
// never its invalidity. The original binary must be left untouched.
func TestRunUpdateInvalidSignatureBlocksInstall(t *testing.T) {
	arch := runtime.GOARCH
	if arch != "amd64" && arch != "arm64" {
		t.Skipf("unsupported test arch %q", arch)
	}
	t.Setenv("HOME", t.TempDir())

	assetName := "theia-linux-" + arch
	binContent := []byte("malicious binary\n")
	sum := sha256.Sum256(binContent)
	checksums := hex.EncodeToString(sum[:]) + "  " + assetName + "\n"

	useHTTPTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"tag_name":"v9.9.9","assets":[`+
				`{"name":%q,"browser_download_url":"https://github.com/dl/%s"},`+
				`{"name":"sha256sums.txt","browser_download_url":"https://github.com/dl/sha256sums.txt"},`+
				`{"name":"sha256sums.txt.sig","browser_download_url":"https://github.com/dl/sha256sums.txt.sig"}]}`,
				assetName, assetName)
		case strings.HasSuffix(r.URL.Path, "sha256sums.txt.sig"):
			_, _ = w.Write([]byte("forged, does not match the embedded public key"))
		case strings.HasSuffix(r.URL.Path, "sha256sums.txt"):
			_, _ = w.Write([]byte(checksums))
		case strings.HasSuffix(r.URL.Path, assetName):
			_, _ = w.Write(binContent)
		default:
			t.Errorf("unexpected request path %s", r.URL.Path)
		}
	})

	exePath := filepath.Join(t.TempDir(), "theia")
	original := []byte("old theia")
	if err := os.WriteFile(exePath, original, 0o755); err != nil {
		t.Fatalf("seeding exe: %v", err)
	}

	buf := &bytes.Buffer{}
	err := runUpdate(context.Background(), buf, exePath, "v0.1.0", false)
	if err == nil {
		t.Fatal("expected an error for an invalid release signature")
	}
	if !strings.Contains(err.Error(), "signature verification failed") {
		t.Errorf("error = %v, want a signature-verification-failed message", err)
	}

	got, readErr := os.ReadFile(exePath)
	if readErr != nil {
		t.Fatalf("reading exe: %v", readErr)
	}
	if string(got) != string(original) {
		t.Errorf("exe was replaced despite a failed signature check: got %q, want %q", got, original)
	}
}
