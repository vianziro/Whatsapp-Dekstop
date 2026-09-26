package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

var updateExecutionMu sync.Mutex

type UIController interface {
	Dispatch(func())
	Eval(string)
}

// appVersion is the single source of truth for the app version. Release
// builds override it with -ldflags "-X main.appVersion=X.Y.Z[.N]"; the literal
// here is only the development fallback. UI strings must never hardcode a
// version — they use the __WA_APP_VERSION__ placeholder replaced at runtime.
var appVersion = "1.5.9.8"

const githubRepo = "vianziro/Whatsapp-Dekstop"

type GitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

type GitHubRelease struct {
	TagName string        `json:"tag_name"`
	Name    string        `json:"name"`
	Body    string        `json:"body"`
	Assets  []GitHubAsset `json:"assets"`
}

type UpdateInfo struct {
	Available      bool   `json:"available"`
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	ReleaseTitle   string `json:"release_title"`
	DownloadURL    string `json:"download_url"`
	AssetSize      int64  `json:"asset_size"`
	// CheckError is non-empty when the version check itself failed (network
	// down, GitHub unreachable). The UI must then say "check failed" instead
	// of the misleading "up to date".
	CheckError string `json:"check_error,omitempty"`
}

func parseVersionParts(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.Split(v, ".")
	res := make([]int, 0, len(parts))
	for _, p := range parts {
		if idx := strings.IndexAny(p, "-+"); idx != -1 {
			p = p[:idx]
		}
		n, err := strconv.Atoi(p)
		if err == nil {
			res = append(res, n)
		} else {
			res = append(res, 0)
		}
	}
	for len(res) < 3 {
		res = append(res, 0)
	}
	return res
}

func isNewerVersion(current, latest string) bool {
	c := parseVersionParts(current)
	l := parseVersionParts(latest)
	maxParts := len(c)
	if len(l) > maxParts {
		maxParts = len(l)
	}
	for len(c) < maxParts {
		c = append(c, 0)
	}
	for len(l) < maxParts {
		l = append(l, 0)
	}
	for i := 0; i < maxParts; i++ {
		if l[i] > c[i] {
			return true
		}
		if l[i] < c[i] {
			return false
		}
	}
	return false
}

func findAssetForCurrentOS(release *GitHubRelease) *GitHubAsset {
	return findAssetForPlatform(release, runtime.GOOS, runtime.GOARCH)
}

// findAssetForOS is retained for platform-agnostic callers and tests. Linux
// amd64 is the historical default; production calls use
// findAssetForCurrentOS so they also include GOARCH.
func findAssetForOS(release *GitHubRelease, goos string) *GitHubAsset {
	return findAssetForPlatform(release, goos, "amd64")
}

func findAssetForPlatform(release *GitHubRelease, goos, goarch string) *GitHubAsset {
	if goos == "darwin" {
		for _, preferred := range []string{"WhatsApp-Desk-macOS-Universal.zip", "WhatsApp-macOS-Universal.zip"} {
			for i := range release.Assets {
				if strings.EqualFold(release.Assets[i].Name, preferred) {
					return &release.Assets[i]
				}
			}
		}
		for _, a := range release.Assets {
			name := strings.ToLower(a.Name)
			if strings.Contains(name, "macos") && strings.HasSuffix(name, ".zip") {
				return &a
			}
		}
	} else if goos == "windows" {
		for _, a := range release.Assets {
			if strings.EqualFold(a.Name, "WhatsAppDesk.exe") {
				return &a
			}
		}
		for _, a := range release.Assets {
			if strings.HasSuffix(strings.ToLower(a.Name), ".exe") {
				return &a
			}
		}
	} else if goos == "linux" {
		// A Linux arm64 process must never receive an x64 binary. Returning no
		// asset is deliberate: the UI can explain that the release is incomplete
		// instead of downloading an executable that cannot start.
		bundleArch := "x64"
		if goarch == "arm64" || goarch == "aarch64" {
			bundleArch = "arm64"
		}
		// On a 4.1-only system (Ubuntu 24.04+, Mint 22.x, Fedora 39+) the
		// historical 4.0-linked tarball cannot start, so prefer the
		// -webkit4.1 artifact when the release carries one. The preference is
		// only computed for the OS actually running this process; cross-OS
		// queries (and the unit tests) keep the historical behavior.
		preferWebkit41 := goos == runtime.GOOS && bundleArch == "x64" && !webkitGTK40Present()
		return findLinuxAsset(release, bundleArch, preferWebkit41)
		// Do not use a broad Linux archive fallback here. It previously selected
		// x64 on arm64 machines merely because it was the only archive present.
	}
	return nil
}

// webkitGTK40Present reports whether the WebKitGTK 4.0 runtime library is
// available on this Linux system.
//
// Two independent signals are consulted, and absence is only reported when the
// filesystem scan proves it:
//
//  1. the dynamic loader cache, via `ldconfig -p`;
//  2. a direct listing of the well-known library directories.
//
// ldconfig is resolved through absolute paths as well as PATH, because a
// GUI-launched app does not always inherit /sbin or /usr/sbin in its PATH —
// desktop launchers and non-systemd sessions routinely omit them. Resolving
// only through PATH made the loader probe fail on exactly the 4.1-only
// systems this check exists for, and the fail-open fallback then silently kept
// selecting the 4.0 artifact.
//
// The fail-open result (reporting 4.0 as present) is reserved for genuinely
// inconclusive cases: no usable ldconfig *and* no readable library directory,
// so no signal could be gathered. It can therefore never be worse than the
// behaviour before this check existed. Non-Linux builds always report true.
func webkitGTK40Present() bool {
	if runtime.GOOS != "linux" {
		return true
	}

	if path := linuxLdconfigPath(); path != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		out, err := exec.CommandContext(ctx, path, "-p").Output()
		cancel()
		if err == nil && strings.Contains(string(out), webkit40LibPrefix) {
			return true
		}
	}

	// Either ldconfig was unavailable, or it reported no match. Scan the
	// library directories as a second opinion: it also covers a 4.0 library
	// that is reachable through LD_LIBRARY_PATH but absent from the cache.
	found, anyReadable := webkit40InDirs(webkit40SearchDirs())
	if found {
		return true
	}
	if anyReadable {
		// Direct filesystem evidence: a readable directory that holds no
		// WebKitGTK 4.0 library, which the loader cache also did not list.
		return false
	}
	// Genuinely inconclusive — no usable ldconfig and no readable library
	// directory — so stay fail-open and keep the historical artifact.
	return true
}

const webkit40LibPrefix = "libwebkit2gtk-4.0.so"

// linuxLdconfigPath resolves the ldconfig binary, preferring PATH and falling
// back to the absolute locations it ships in. Returns "" when it cannot be
// found or is not executable.
func linuxLdconfigPath() string {
	candidates := make([]string, 0, 5)
	if path, err := exec.LookPath("ldconfig"); err == nil && path != "" {
		candidates = append(candidates, path)
	}
	candidates = append(candidates,
		"/sbin/ldconfig",
		"/usr/sbin/ldconfig",
		"/bin/ldconfig",
		"/usr/bin/ldconfig",
	)
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() || info.Mode()&0111 == 0 {
			continue
		}
		return candidate
	}
	return ""
}

// webkit40SearchDirs lists the directories WebKitGTK 4.0 is installed into by
// the distributions this project targets, including the usrmerge symlinks and
// the Fedora multiarch location.
func webkit40SearchDirs() []string {
	return []string{
		"/usr/lib/x86_64-linux-gnu",
		"/lib/x86_64-linux-gnu",
		"/usr/lib/aarch64-linux-gnu",
		"/lib/aarch64-linux-gnu",
		"/usr/lib64",
		"/usr/lib",
		"/lib",
	}
}

// webkit40InDirs reports whether any of dirs contains a WebKitGTK 4.0 shared
// object, and whether at least one directory could be read. The second value
// is what makes a negative first value meaningful: a readable directory that
// holds no such library is evidence of absence, while a set of unreadable
// directories proves nothing.
func webkit40InDirs(dirs []string) (found, anyReadable bool) {
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		anyReadable = true
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), webkit40LibPrefix) {
				return true, true
			}
		}
	}
	return false, anyReadable
}

// findLinuxAsset selects the self-update tarball for a Linux bundleArch.
// When preferWebkit41 is set, the -webkit4.1 artifact wins if present and the
// historical 4.0-named tarball remains the fallback, so releases that predate
// the 4.1 variant keep working. Cross-arch fallback is still refused: an
// arm64 request never receives an x64 binary.
func findLinuxAsset(release *GitHubRelease, bundleArch string, preferWebkit41 bool) *GitHubAsset {
	names := []string{
		"WhatsApp-Desk-Linux-" + bundleArch + ".tar.gz",
		"WhatsApp-Linux-" + bundleArch + ".tar.gz",
	}
	if preferWebkit41 {
		names = append([]string{"WhatsApp-Desk-Linux-" + bundleArch + "-webkit4.1.tar.gz"}, names...)
	}
	for _, preferred := range names {
		for i := range release.Assets {
			if strings.EqualFold(release.Assets[i].Name, preferred) {
				return &release.Assets[i]
			}
		}
	}
	return nil
}

func updateDownloadExtension(downloadURL string) string {
	parsed, err := url.Parse(downloadURL)
	if err == nil {
		path := strings.ToLower(parsed.Path)
		if strings.HasSuffix(path, ".tar.gz") {
			return ".tar.gz"
		}
		if ext := filepath.Ext(path); ext == ".zip" || ext == ".exe" || ext == ".tgz" {
			return ext
		}
	}
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	if runtime.GOOS == "linux" {
		return ".tar.gz"
	}
	return ".zip"
}

// updateAssetForOS is the stable release-latest download URL. Following it
// returns an HTTP 302 whose Location header names the newest release asset,
// which is all the version check needs — no REST API call, so the 60-requests-
// per-hour anonymous GitHub API limit can never blind the updater again.
func updateAssetForPlatform(goos, goarch string) string {
	switch goos {
	case "darwin":
		return "https://github.com/" + githubRepo + "/releases/latest/download/WhatsApp-Desk-macOS-Universal.zip"
	case "windows":
		return "https://github.com/" + githubRepo + "/releases/latest/download/WhatsAppDesk.exe"
	case "linux":
		if goarch == "arm64" || goarch == "aarch64" {
			return "https://github.com/" + githubRepo + "/releases/latest/download/WhatsApp-Desk-Linux-arm64.tar.gz"
		}
		// Primary update path uses this static URL (no API asset list), so it
		// needs the same variant preference: on a 4.1-only system the plain
		// 4.0-linked tarball cannot start. arm64 has no -webkit4.1 artifact,
		// and cross-OS queries keep the historical URL.
		if goos == runtime.GOOS && !webkitGTK40Present() {
			return "https://github.com/" + githubRepo + "/releases/latest/download/WhatsApp-Desk-Linux-x64-webkit4.1.tar.gz"
		}
		return "https://github.com/" + githubRepo + "/releases/latest/download/WhatsApp-Desk-Linux-x64.tar.gz"
	}
	return ""
}

func updateAssetForOS(goos string) string {
	return updateAssetForPlatform(goos, "amd64")
}

// versionFromAssetName extracts the version embedded in a release asset name,
// e.g. "WhatsApp-Desk-macOS-Universal.zip" names carry no version, so the
// check resolves the /releases/latest page tag instead. Kept for API fallback.
func versionFromTagName(tag string) string {
	return strings.TrimPrefix(strings.TrimSpace(tag), "v")
}

func checkForUpdate(currentVer string) (*UpdateInfo, error) {
	start := time.Now()
	info := &UpdateInfo{CurrentVersion: currentVer}

	// Primary path: resolve the latest release tag via the release page
	// redirect chain (github.com/.../releases/latest -> .../tag/vX.Y.Z).
	// This is a plain web redirect, not the REST API: no rate limit.
	latestVer, err := fetchLatestVersionViaRedirect()
	if err == nil && latestVer != "" {
		cacheDebugLog("update-check ok in %s: current=%s latest=%s",
			time.Since(start).Round(time.Millisecond), currentVer, latestVer)
		info.LatestVersion = latestVer
		if isNewerVersion(currentVer, latestVer) {
			assetURL := updateAssetForPlatform(runtime.GOOS, runtime.GOARCH)
			if assetURL == "" {
				return info, fmt.Errorf("no compatible %s update asset configured", runtime.GOOS)
			}
			info.Available = true
			info.DownloadURL = assetURL
		}
		return info, nil
	}

	// Fallback: REST API (rate-limited to 60/hr per IP for anonymous users).
	apiInfo, apiErr := checkForUpdateViaAPI(currentVer)
	if apiErr != nil {
		cacheDebugLog("update-check FAILED in %s: redirect=%v api=%v",
			time.Since(start).Round(time.Millisecond), err, apiErr)
		return info, fmt.Errorf("version check failed (redirect: %v; api: %v)", err, apiErr)
	}
	cacheDebugLog("update-check ok (api fallback) in %s: latest=%s",
		time.Since(start).Round(time.Millisecond), apiInfo.LatestVersion)
	return apiInfo, nil
}

// fetchLatestVersionViaRedirect follows the releases/latest page redirect and
// reads the target tag out of the final URL path (.../tag/v1.2.3).
func fetchLatestVersionViaRedirect() (string, error) {
	pageURL := "https://github.com/" + githubRepo + "/releases/latest"
	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "WhatsAppDesk-Updater")

	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // stop at the first 302
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// CheckRedirect returns ErrUseLastResponse, so a healthy redirect arrives
	// here as 30x with a Location header; anything else is a failure.
	loc := resp.Header.Get("Location")
	if resp.StatusCode < 300 || resp.StatusCode >= 400 || loc == "" {
		return "", fmt.Errorf("releases/latest returned HTTP %d", resp.StatusCode)
	}

	// Location: https://github.com/OWNER/REPO/releases/tag/v1.2.3
	segments := strings.Split(loc, "/")
	for i := len(segments) - 1; i > 0; i-- {
		if segments[i-1] == "tag" {
			return versionFromTagName(segments[i]), nil
		}
	}
	return "", fmt.Errorf("unexpected redirect target %q", loc)
}

func checkForUpdateViaAPI(currentVer string) (*UpdateInfo, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "WhatsAppDesk-Updater")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("GitHub API rate limit reached (HTTP 403)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var rel GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}

	latestVer := versionFromTagName(rel.TagName)
	info := &UpdateInfo{
		CurrentVersion: currentVer,
		LatestVersion:  latestVer,
		ReleaseTitle:   rel.Name,
	}

	if isNewerVersion(currentVer, latestVer) {
		asset := findAssetForCurrentOS(&rel)
		if asset == nil || strings.TrimSpace(asset.BrowserDownloadURL) == "" {
			return info, fmt.Errorf("release %s has no compatible %s update asset", latestVer, runtime.GOOS)
		}
		info.Available = true
		info.DownloadURL = asset.BrowserDownloadURL
		info.AssetSize = asset.Size
	}

	return info, nil
}

type progressWriter struct {
	total      int64
	downloaded int64
	lastPct    int
	onProgress func(percent int)
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.downloaded += int64(n)
	if pw.total > 0 && pw.onProgress != nil {
		pct := int((pw.downloaded * 100) / pw.total)
		if pct != pw.lastPct {
			pw.lastPct = pct
			pw.onProgress(pct)
		}
	} else if pw.onProgress != nil {
		// Fallback when Content-Length is missing or chunked
		mb := float64(pw.downloaded) / (1024 * 1024)
		pct := int(mb * 10)
		if pct > 95 {
			pct = 95
		}
		if pct != pw.lastPct {
			pw.lastPct = pct
			pw.onProgress(pct)
		}
	}
	return n, nil
}

// maxUpdateDownloadBytes bounds self-update downloads. Genuine release
// artifacts are tens of megabytes; anything far beyond that is either a
// compromised endpoint or a disk-fill attempt, never a legitimate update.
var maxUpdateDownloadBytes int64 = 512 << 20

func downloadFileWithProgress(url, destPath string, onProgress func(int)) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "WhatsAppDesk-Updater")

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with HTTP %d", resp.StatusCode)
	}
	// Fast path: a truthful Content-Length above the cap fails before a
	// single byte hits the disk.
	if resp.ContentLength > maxUpdateDownloadBytes {
		return fmt.Errorf("download rejected: declared size %d exceeds %d-byte limit", resp.ContentLength, maxUpdateDownloadBytes)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	pw := &progressWriter{
		total:      resp.ContentLength,
		onProgress: onProgress,
	}

	// Slow path: Content-Length may lie or be absent (chunked), so cap the
	// body itself. The +1 detects overflow; a partial file never survives.
	// (Closed explicitly before Remove: Windows cannot delete an open file.)
	n, err := io.Copy(out, io.LimitReader(io.TeeReader(resp.Body, pw), maxUpdateDownloadBytes+1))
	if err != nil {
		_ = out.Close()
		_ = os.Remove(destPath)
		return err
	}
	if n > maxUpdateDownloadBytes {
		_ = out.Close()
		_ = os.Remove(destPath)
		return fmt.Errorf("download rejected: size exceeds %d-byte limit", maxUpdateDownloadBytes)
	}
	return nil
}

// windowsUpdateBatch is kept platform-neutral so the restart contract can be
// regression-tested on every development host. The batch itself runs only in
// the Windows helper after the GUI process exits.
func windowsUpdateBatch(pid int, newExePath, execPath string) string {
	return fmt.Sprintf(`@echo off
setlocal
set OLD_PID=%d
set SRC=%s
set DST=%s
set UPDATE_LOG=%%TEMP%%\WhatsAppDesk-update.log

:wait_loop
tasklist /fi "PID eq %%OLD_PID%%" 2>NUL | findstr /i "%%OLD_PID%%" >NUL
if not errorlevel 1 (
    ping -n 2 127.0.0.1 >NUL
    goto wait_loop
)

ping -n 2 127.0.0.1 >NUL

set RETRY=0
:copy_loop
copy /y "%%SRC%%" "%%DST%%" >NUL 2>&1
if not errorlevel 1 goto restart
set /a RETRY+=1
if %%RETRY%% leq 12 (
    ping -n 2 127.0.0.1 >NUL
    goto copy_loop
)
goto copy_failed

:copy_failed
> "%%UPDATE_LOG%%" echo WhatsApp Desk update could not replace "%%DST%%". The existing version was restarted.
start "" "%%DST%%"
goto cleanup

:restart
del /f /q "%%SRC%%" >NUL 2>&1
start "" "%%DST%%"

:cleanup
del /f /q "%%~f0" >NUL 2>&1
`, pid, newExePath, execPath)
}

// isAllowedUpdateURL reports whether downloadURL is a legitimate self-update
// payload location. The URL arrives via the JS bridge (startUpdateNative), so
// it must never be trusted blindly: only release artifacts of this repository
// served over HTTPS from github.com are accepted. Redirects to GitHub's own
// asset CDN are followed later by the downloader itself, so the bridge never
// needs to accept any other host.
func isAllowedUpdateURL(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u == nil {
		return false
	}
	if !strings.EqualFold(u.Scheme, "https") || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host != "github.com" {
		return false
	}
	// e.g. /vianziro/Whatsapp-Dekstop/releases/download/v1.2.3/...
	// or /vianziro/Whatsapp-Dekstop/releases/latest/download/...
	return strings.HasPrefix(strings.ToLower(u.Path), "/vianziro/whatsapp-dekstop/releases/")
}

func executeUpdate(ui UIController, downloadURL string) error {
	if !updateExecutionMu.TryLock() {
		ui.Dispatch(func() {
			ui.Eval("if (window.onUpdateStatus) { window.onUpdateStatus('Update already in progress...'); }")
		})
		return nil
	}
	defer updateExecutionMu.Unlock()

	// The download URL is attacker-reachable through the JS bridge, so reject
	// anything outside our own GitHub release artifacts before any network
	// or filesystem side effect happens.
	if !isAllowedUpdateURL(downloadURL) {
		err := fmt.Errorf("update rejected: URL is not a WhatsApp Desk GitHub release artifact")
		ui.Dispatch(func() {
			ui.Eval(fmt.Sprintf("if (window.onUpdateError) { window.onUpdateError(%q); }", err.Error()))
		})
		return err
	}

	ext := updateDownloadExtension(downloadURL)
	destFile := filepath.Join(os.TempDir(), "whatsapp_update_download"+ext)
	_ = os.Remove(destFile)
	defer os.Remove(destFile)

	ui.Dispatch(func() {
		ui.Eval("if (window.onUpdateStatus) { window.onUpdateStatus('Downloading update... 0%'); }")
	})

	err := downloadFileWithProgress(downloadURL, destFile, func(pct int) {
		ui.Dispatch(func() {
			ui.Eval(fmt.Sprintf("if (window.onUpdateProgress) { window.onUpdateProgress(%d); }", pct))
		})
	})
	if err != nil {
		ui.Dispatch(func() {
			ui.Eval(fmt.Sprintf("if (window.onUpdateError) { window.onUpdateError(%q); }", err.Error()))
		})
		return err
	}

	// Integrity gate: the downloaded artifact must match the SHA-256 published
	// for this release before anything is executed or installed.
	assetName := path.Base(downloadURL)
	if i := strings.IndexAny(assetName, "?#"); i != -1 {
		assetName = assetName[:i]
	}
	ui.Dispatch(func() {
		ui.Eval("if (window.onUpdateStatus) { window.onUpdateStatus('Verifying download...'); }")
	})
	if err := verifyDownloadedChecksum(destFile, assetName, releaseBaseURLForAsset(downloadURL), checksumVerificationRequired); err != nil {
		ui.Dispatch(func() {
			ui.Eval(fmt.Sprintf("if (window.onUpdateError) { window.onUpdateError(%q); }", err.Error()))
		})
		return err
	}

	ui.Dispatch(func() {
		ui.Eval("if (window.onUpdateStatus) { window.onUpdateStatus('Installing update & restarting...'); }")
	})

	time.Sleep(600 * time.Millisecond)
	err = applyUpdate(destFile)
	if err != nil {
		ui.Dispatch(func() {
			ui.Eval(fmt.Sprintf("if (window.onUpdateError) { window.onUpdateError(%q); }", err.Error()))
		})
		return err
	}
	return nil
}
