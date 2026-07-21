package cmd

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Elysium-Labs-EU/theia/internal/buildinfo"
	"github.com/Elysium-Labs-EU/theia/internal/ui"
	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
)

const theiaRepo = "Elysium-Labs-EU/theia"

const theiaService = "theia.service"

var httpClient = &http.Client{
	Timeout: 15 * time.Second,
}

// updateUserAgent is sent on every request to the GitHub release API and asset
// downloads. The GitHub REST API rejects requests without a User-Agent with a
// 403, unlike the Gitea/Codeberg API this updater previously targeted.
const updateUserAgent = "theia-updater"

// releaseSigningPublicKeyPEM is the ECDSA P-256 public key (SubjectPublicKeyInfo,
// PEM) used to verify the detached signature over each release's
// sha256sums.txt. The matching private key lives only as the
// RELEASE_SIGNING_KEY secret in GitHub Actions and is used by
// .github/workflows/build-and-release.yml to sign at release time — it is
// never checked into this repo.
const releaseSigningPublicKeyPEM = `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEld+PbyOPIKYMhIHdUBTa0SsMVTNG
ueCARCU4EJIMNNKwWAh9FgC7wAZbrbBRfoPpv0EH4d3m9Sc2obONMw8aGw==
-----END PUBLIC KEY-----
`

// requireReleaseSignature gates whether a release with no sha256sums.txt.sig
// asset is refused outright rather than merely warned about. Keep this false
// until the RELEASE_SIGNING_KEY secret is provisioned in GitHub Actions and
// the first signed release has shipped — flipping it before then would make
// every existing release refuse to install. Once a signed release exists,
// flip to true so an unsigned or signature-stripped release can no longer be
// installed silently.
const requireReleaseSignature = false

// Asset is one file attached to a GitHub release.
type Asset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
}

// Release is the subset of GitHub's release API response theia needs.
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// AssetFor returns the release asset for theia on linux/arch.
func (r Release) AssetFor(arch string) (Asset, bool) {
	want := fmt.Sprintf("theia-linux-%s", arch)
	for _, a := range r.Assets {
		if a.Name == want {
			return a, true
		}
	}
	return Asset{}, false
}

// ChecksumsAsset returns the sha256sums.txt asset, if the release has one.
func (r Release) ChecksumsAsset() (Asset, bool) {
	for _, a := range r.Assets {
		if a.Name == "sha256sums.txt" {
			return a, true
		}
	}
	return Asset{}, false
}

// SignatureAsset returns the sha256sums.txt.sig asset — a detached ECDSA
// signature over sha256sums.txt — if the release has one. Soft-checked by
// requireReleaseSignature.
func (r Release) SignatureAsset() (Asset, bool) {
	for _, a := range r.Assets {
		if a.Name == "sha256sums.txt.sig" {
			return a, true
		}
	}
	return Asset{}, false
}

// fetchLatestRelease fetches the latest theia release from GitHub.
// GitHub's "latest" endpoint only ever returns stable (non-prerelease,
// non-draft) releases, so when includePre is true this instead lists all
// releases (newest first) and returns the first one — the only way to reach
// a release while every published version is still a pre-release.
func fetchLatestRelease(ctx context.Context, includePre bool) (Release, error) {
	reqURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", theiaRepo)
	if includePre {
		reqURL = fmt.Sprintf("https://api.github.com/repos/%s/releases", theiaRepo)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return Release{}, fmt.Errorf("building release request: %w", err)
	}
	req.Header.Set("User-Agent", updateUserAgent)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := httpClient.Do(req) // #nosec G704 -- URL is constructed from a hardcoded GitHub API base, not user input
	if err != nil {
		return Release{}, fmt.Errorf("fetching latest release: %w", err)
	}
	if resp == nil {
		return Release{}, fmt.Errorf("fetching latest release: nil response")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("fetching latest release: unexpected status %s", resp.Status)
	}

	if includePre {
		var releases []Release
		if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
			return Release{}, fmt.Errorf("decoding release response: %w", err)
		}
		if len(releases) == 0 {
			return Release{}, fmt.Errorf("no releases found")
		}
		return releases[0], nil
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return Release{}, fmt.Errorf("decoding release response: %w", err)
	}
	return rel, nil
}

// downloadFile fetches downloadURL to destPath. It refuses anything but a
// plain https://github.com URL, since this is used to fetch and then
// execute-in-place a new theia binary.
func downloadFile(ctx context.Context, downloadURL, destPath string) error {
	if err := validateDownloadHost(downloadURL); err != nil {
		return err
	}

	resp, err := fetchDownload(ctx, downloadURL)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	return writeDownloadBody(resp, destPath, downloadURL)
}

// validateDownloadHost refuses anything but a plain https://github.com URL.
func validateDownloadHost(downloadURL string) error {
	u, err := url.Parse(downloadURL)
	if err != nil {
		return fmt.Errorf("parsing download URL: %w", err)
	}
	if u.Scheme != "https" || u.Host != "github.com" {
		return fmt.Errorf("refusing to download from untrusted host %q", u.Host)
	}
	return nil
}

// fetchDownload issues the GET request for downloadURL and validates the
// response status. The caller owns closing the returned response body.
func fetchDownload(ctx context.Context, downloadURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building download request: %w", err)
	}
	req.Header.Set("User-Agent", updateUserAgent)

	resp, err := httpClient.Do(req) // #nosec G704 -- downloadURL is validated above to be https://github.com
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", downloadURL, err)
	}
	if resp == nil {
		return nil, fmt.Errorf("downloading %s: nil response", downloadURL)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("downloading %s: unexpected status %s", downloadURL, resp.Status)
	}
	return resp, nil
}

// writeDownloadBody streams resp's body to destPath and checks the byte
// count against the response's declared content length, if any.
func writeDownloadBody(resp *http.Response, destPath, downloadURL string) error {
	out, err := os.Create(destPath) //nolint:gosec // destPath is a caller-controlled temp path
	if err != nil {
		return fmt.Errorf("creating %s: %w", destPath, err)
	}
	defer func() { _ = out.Close() }()

	n, err := io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("writing %s: %w", destPath, err)
	}
	if resp.ContentLength > 0 && n != resp.ContentLength {
		return fmt.Errorf("downloading %s: got %d bytes, expected %d", downloadURL, n, resp.ContentLength)
	}
	return nil
}

// verifyChecksum checks binaryPath's sha256 against the entry for assetName
// in a sha256sums.txt file's contents (the standard `sha256sum` output
// format: "<hex digest>  <filename>" per line).
func verifyChecksum(binaryPath, checksumsContent, assetName string) error {
	var want string
	for line := range strings.SplitSeq(checksumsContent, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == assetName {
			want = fields[0]
			break
		}
	}
	if want == "" {
		return fmt.Errorf("no checksum entry for %s", assetName)
	}

	f, err := os.Open(binaryPath) //nolint:gosec // binaryPath is a caller-controlled temp path
	if err != nil {
		return fmt.Errorf("opening %s: %w", binaryPath, err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hashing %s: %w", binaryPath, err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", assetName, want, got)
	}
	return nil
}

// parseReleaseSigningPublicKey decodes the embedded release signing public
// key. Pure — no I/O.
func parseReleaseSigningPublicKey() (*ecdsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(releaseSigningPublicKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("decoding embedded release signing public key: no PEM block found")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing embedded release signing public key: %w", err)
	}
	ecdsaPub, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("embedded release signing public key is %T, want ECDSA", pub)
	}
	return ecdsaPub, nil
}

// verifySignature checks sig — an ASN.1 DER ECDSA signature, as produced by
// `openssl dgst -sha256 -sign` — against the SHA-256 digest of data, using
// pub. Pure — no I/O.
func verifySignature(pub *ecdsa.PublicKey, data, sig []byte) error {
	digest := sha256.Sum256(data)
	if !ecdsa.VerifyASN1(pub, digest[:], sig) {
		return fmt.Errorf("signature does not match")
	}
	return nil
}

// verifyChecksumsSignature checks sig against checksumsData using the
// embedded release signing public key. Pure — no I/O.
func verifyChecksumsSignature(checksumsData, sig []byte) error {
	pub, err := parseReleaseSigningPublicKey()
	if err != nil {
		return err
	}
	if err := verifySignature(pub, checksumsData, sig); err != nil {
		return fmt.Errorf("signature does not match sha256sums.txt")
	}
	return nil
}

// verifyReleaseSignature downloads rel's sha256sums.txt.sig into tmpDir and
// verifies it against checksumsData, writing a status line to out either
// way.
//
// A release with no signature asset is only a hard error once
// requireReleaseSignature is true (see its doc comment for the rollout
// plan); until then it's a warning, since sha256 checksum verification
// alone has already run by the time this is called. A signature asset that
// fails to verify is always a hard error — that's a stronger integrity
// signal than "no signature was ever published", so it's never soft-failed.
func verifyReleaseSignature(ctx context.Context, out io.Writer, rel Release, checksumsData []byte, tmpDir string) error {
	sigAsset, ok := rel.SignatureAsset()
	if !ok {
		if requireReleaseSignature {
			return &ui.UserError{Err: fmt.Errorf("release %s has no sha256sums.txt.sig", rel.TagName)}
		}
		_, _ = fmt.Fprintf(out, "%s release %s has no signature (sha256sums.txt.sig) — checksum-only integrity\n", ui.LabelWarning.Render("warning"), rel.TagName)
		return nil
	}

	sigTmp := filepath.Join(tmpDir, "sha256sums.txt.sig")
	if dlErr := downloadFile(ctx, sigAsset.DownloadURL, sigTmp); dlErr != nil {
		return fmt.Errorf("downloading signature: %w", dlErr)
	}
	sigData, err := os.ReadFile(sigTmp) //nolint:gosec // fixed name in a theia-owned temp dir
	if err != nil {
		return fmt.Errorf("reading signature: %w", err)
	}

	if verifyErr := verifyChecksumsSignature(checksumsData, sigData); verifyErr != nil {
		return &ui.UserError{Err: fmt.Errorf("signature verification failed for %s: %w — refusing to install", rel.TagName, verifyErr)}
	}
	_, _ = fmt.Fprintf(out, "%s signature verified\n", ui.LabelSuccess.Render("✓"))
	return nil
}

// copyFile copies src to dst, creating or truncating dst, preserving src's
// file mode.
func copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}

	in, err := os.Open(src) //nolint:gosec // caller-controlled paths
	if err != nil {
		return fmt.Errorf("opening %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode()) //nolint:gosec // caller-controlled paths
	if err != nil {
		return fmt.Errorf("creating %s: %w", dst, err)
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copying to %s: %w", dst, err)
	}
	return nil
}

// replaceBinary installs newPath over dstPath, which may be the currently
// running executable: it copies to a same-directory temp file, chmods it
// executable, then renames over dstPath. The rename is atomic on the same
// filesystem, and the OS keeps the old inode open for any process (e.g.
// the one calling this function, or a systemd service still shutting down)
// that's already running it.
func replaceBinary(newPath, dstPath string) error {
	tmp := dstPath + ".tmp"
	if err := copyFile(newPath, tmp); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o755); err != nil { //nolint:gosec // installed binary must be executable
		_ = os.Remove(tmp)
		return fmt.Errorf("chmod %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, dstPath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("installing %s: %w", dstPath, err)
	}
	return nil
}

// checkWritable verifies dir is writable by creating and removing a probe
// file in it.
func checkWritable(dir string) error {
	probe := filepath.Join(dir, ".theia-write-check")
	f, err := os.Create(probe) //nolint:gosec // fixed probe filename in a caller-controlled dir
	if err != nil {
		return err
	}
	_ = f.Close()
	return os.Remove(probe)
}

// currentBinaryPath returns the resolved path of the running theia binary.
func currentBinaryPath() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locating running binary: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return "", fmt.Errorf("resolving running binary path: %w", err)
	}
	return exePath, nil
}

// hostArch maps runtime.GOARCH to the arch suffix used in release asset
// names (see install.sh's detect_arch).
func hostArch() (string, error) {
	switch runtime.GOARCH {
	case "amd64", "arm64":
		return runtime.GOARCH, nil
	default:
		return "", &ui.UserError{Err: fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)}
	}
}

// normalizeSemver prefixes a bare "0.0.1"-style version with "v" so it's
// valid input for golang.org/x/mod/semver, which requires the "v" prefix.
func normalizeSemver(v string) string {
	if v != "" && v[0] != 'v' {
		return "v" + v
	}
	return v
}

// serviceIsActive reports whether theia.service is currently running under
// systemd. Non-systemd environments (dev machines, containers, tests) fail
// this check harmlessly and are treated as "no service to stop."
func serviceIsActive(ctx context.Context) bool {
	return exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", theiaService).Run() == nil
}

// stopService stops theia.service before the binary underneath it gets
// replaced. Without this, overwriting the binary in place while systemd
// still has it open for execution fails with "text file busy" on some
// filesystems, and even where it doesn't, the running process keeps serving
// off the old (now unlinked) inode until it's restarted anyway.
func stopService(ctx context.Context, out io.Writer) bool {
	if !serviceIsActive(ctx) {
		return false
	}
	if err := exec.CommandContext(ctx, "systemctl", "stop", theiaService).Run(); err != nil {
		_, _ = fmt.Fprintf(out, "%s could not stop %s: %v\n", ui.LabelWarning.Render("warning"), theiaService, err)
		return false
	}
	_, _ = fmt.Fprintf(out, "%s stopped %s\n", ui.TextMuted.Render("i"), theiaService)
	return true
}

// startService restarts theia.service after a binary swap. Only called when
// stopService reported it actually stopped a running instance.
func startService(ctx context.Context, out io.Writer) {
	if err := exec.CommandContext(ctx, "systemctl", "start", theiaService).Run(); err != nil {
		_, _ = fmt.Fprintf(out, "%s could not restart %s: %v\n", ui.LabelWarning.Render("warning"), theiaService, err)
		_, _ = fmt.Fprintf(out, "  %s %s\n", ui.TextMuted.Render("run:"), ui.TextCommand.Render("sudo systemctl start theia"))
		return
	}
	_, _ = fmt.Fprintf(out, "%s restarted %s\n", ui.LabelSuccess.Render("✓"), theiaService)
}

// runUpdate implements `theia system update` against an explicit exePath, so it
// can be exercised in tests without touching the test binary itself
// (os.Executable() under `go test` is the test binary).
func runUpdate(ctx context.Context, out io.Writer, exePath, currentVersion string, includePre bool) error {
	rel, latestVer, err := resolveLatestRelease(ctx, includePre)
	if err != nil {
		return err
	}

	if isAlreadyLatest(currentVersion, latestVer) {
		_, _ = fmt.Fprintf(out, "%s already on the latest version (%s)\n", ui.LabelSuccess.Render("✓"), currentVersion)
		return nil
	}
	_, _ = fmt.Fprintf(out, "%s new version available: %s -> %s\n", ui.LabelInfo.Render("i"), currentVersion, latestVer)

	asset, checksums, err := selectUpdateAssets(rel, latestVer)
	if err != nil {
		return err
	}

	if writeErr := checkWritable(filepath.Dir(exePath)); writeErr != nil {
		return &ui.UserError{
			Err:  fmt.Errorf("%s is not writable: %w", filepath.Dir(exePath), writeErr),
			Hint: "sudo theia system update",
		}
	}

	tmpDir, err := os.MkdirTemp("", "theia-update")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	binTmp, err := downloadAndVerifyUpdate(ctx, out, tmpDir, latestVer, rel, asset, checksums)
	if err != nil {
		return err
	}

	return installUpdate(ctx, out, exePath, binTmp, currentVersion, latestVer)
}

// resolveLatestRelease checks GitHub for the latest (or latest including
// pre-release) theia release and returns it alongside its tag name.
func resolveLatestRelease(ctx context.Context, includePre bool) (Release, string, error) {
	var rel Release
	err := ui.WithSpinner("Checking for updates...", func() error {
		var err error
		rel, err = fetchLatestRelease(ctx, includePre)
		return err
	})
	if err != nil {
		return Release{}, "", fmt.Errorf("checking for updates: %w", err)
	}
	return rel, rel.TagName, nil
}

// isAlreadyLatest reports whether currentVersion is already at or ahead of
// latestVer. Non-semver versions (e.g. dev builds) are treated as not
// up to date so an update always proceeds.
func isAlreadyLatest(currentVersion, latestVer string) bool {
	currentVer := normalizeSemver(currentVersion)
	return semver.IsValid(currentVer) && semver.IsValid(latestVer) && semver.Compare(currentVer, latestVer) >= 0
}

// selectUpdateAssets picks the platform binary and checksums file out of
// rel, failing with a user-facing error if either is missing.
func selectUpdateAssets(rel Release, latestVer string) (Asset, Asset, error) {
	arch, err := hostArch()
	if err != nil {
		return Asset{}, Asset{}, err
	}
	asset, ok := rel.AssetFor(arch)
	if !ok {
		return Asset{}, Asset{}, &ui.UserError{Err: fmt.Errorf("release %s has no asset for linux-%s", latestVer, arch)}
	}
	checksums, ok := rel.ChecksumsAsset()
	if !ok {
		return Asset{}, Asset{}, &ui.UserError{Err: fmt.Errorf("release %s is missing sha256sums.txt", latestVer)}
	}
	return asset, checksums, nil
}

// downloadAndVerifyUpdate downloads the release binary and its checksums
// file into tmpDir and verifies the binary's checksum, returning the path
// to the verified binary.
func downloadAndVerifyUpdate(ctx context.Context, out io.Writer, tmpDir, latestVer string, rel Release, asset, checksums Asset) (string, error) {
	binTmp := filepath.Join(tmpDir, "theia")
	err := ui.WithSpinner(fmt.Sprintf("Downloading %s...", latestVer), func() error {
		return downloadFile(ctx, asset.DownloadURL, binTmp)
	})
	if err != nil {
		return "", fmt.Errorf("downloading update: %w", err)
	}

	checksumsTmp := filepath.Join(tmpDir, "sha256sums.txt")
	if dlErr := downloadFile(ctx, checksums.DownloadURL, checksumsTmp); dlErr != nil {
		return "", fmt.Errorf("downloading checksums: %w", dlErr)
	}
	checksumsData, err := os.ReadFile(checksumsTmp) //nolint:gosec // fixed name in a theia-owned temp dir
	if err != nil {
		return "", fmt.Errorf("reading checksums: %w", err)
	}
	if verifyErr := verifyChecksum(binTmp, string(checksumsData), asset.Name); verifyErr != nil {
		return "", &ui.UserError{Err: verifyErr}
	}
	_, _ = fmt.Fprintf(out, "%s checksum verified\n", ui.LabelSuccess.Render("✓"))

	if sigErr := verifyReleaseSignature(ctx, out, rel, checksumsData, tmpDir); sigErr != nil {
		return "", sigErr
	}

	return binTmp, nil
}

// installUpdate backs up the current binary, stops theia.service if it's
// running, swaps in binTmp, and restarts the service if it was stopped.
func installUpdate(ctx context.Context, out io.Writer, exePath, binTmp, currentVersion, latestVer string) error {
	backupPath := exePath + ".backup"
	if backupErr := copyFile(exePath, backupPath); backupErr != nil {
		_, _ = fmt.Fprintf(out, "%s could not create backup of the current binary: %v\n", ui.LabelWarning.Render("warning"), backupErr)
	} else {
		_, _ = fmt.Fprintf(out, "%s backed up current binary to %s\n", ui.TextMuted.Render("i"), backupPath)
	}

	wasRunning := stopService(ctx, out)

	if replaceErr := replaceBinary(binTmp, exePath); replaceErr != nil {
		if wasRunning {
			startService(ctx, out)
		}
		return fmt.Errorf("installing new binary: %w", replaceErr)
	}

	if wasRunning {
		startService(ctx, out)
	}

	_, _ = fmt.Fprintf(out, "%s updated %s -> %s\n", ui.LabelSuccess.Render("✓"), currentVersion, latestVer)
	return nil
}

func newUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update",
		Short:   "Download and install the latest theia release",
		Example: "  theia system update        # check and apply latest stable release\n  theia system update --pre  # include pre-releases",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Flags parsed fine to reach here, so any error from this point
			// on is a runtime failure, not a usage mistake — don't dump the
			// flags/usage block for it.
			cmd.SilenceUsage = true

			exePath, err := currentBinaryPath()
			if err != nil {
				return err
			}
			includePre, err := cmd.Flags().GetBool("pre")
			if err != nil {
				return err
			}
			return runUpdate(cmd.Context(), cmd.OutOrStdout(), exePath, buildinfo.GetVersionOnly(), includePre)
		},
	}
	cmd.Flags().Bool("pre", false, "include pre-releases in update check")
	return cmd
}
