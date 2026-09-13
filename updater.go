package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
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
var appVersion = "1.5.9.2"

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
		for _, preferred := range []string{
			"WhatsApp-Desk-Linux-" + bundleArch + ".tar.gz",
			"WhatsApp-Linux-" + bundleArch + ".tar.gz",
		} {
			for i := range release.Assets {
				if strings.EqualFold(release.Assets[i].Name, preferred) {
					return &release.Assets[i]
				}
			}
		}
		// Do not use a broad Linux archive fallback here. It previously selected
		// x64 on arm64 machines merely because it was the only archive present.
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

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	pw := &progressWriter{
		total:      resp.ContentLength,
		onProgress: onProgress,
	}

	_, err = io.Copy(out, io.TeeReader(resp.Body, pw))
	return err
}

func executeUpdate(ui UIController, downloadURL string) error {
	if !updateExecutionMu.TryLock() {
		ui.Dispatch(func() {
			ui.Eval("if (window.onUpdateStatus) { window.onUpdateStatus('Update already in progress...'); }")
		})
		return nil
	}
	defer updateExecutionMu.Unlock()

	ext := updateDownloadExtension(downloadURL)
	destFile := filepath.Join(os.TempDir(), "whatsapp_update_download"+ext)
	_ = os.Remove(destFile)

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
