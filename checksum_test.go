package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubRoundTripper serves a canned SHA256SUMS body (or a 404) so the HTTP path
// is exercised without binding a port, which sandboxed test runs forbid.
type stubRoundTripper struct {
	body       string
	statusCode int
}

func (s stubRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	code := s.statusCode
	if code == 0 {
		code = http.StatusOK
	}
	return &http.Response{
		StatusCode: code,
		Body:       io.NopCloser(strings.NewReader(s.body)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

// withStubTransport swaps the package HTTP client for the duration of a test.
func withStubTransport(t *testing.T, body string, statusCode int) {
	t.Helper()
	prev := checksumHTTPTransport
	checksumHTTPTransport = stubRoundTripper{body: body, statusCode: statusCode}
	t.Cleanup(func() { checksumHTTPTransport = prev })
}

func TestParseChecksumFileGNUStyle(t *testing.T) {
	sum := strings.Repeat("ab", 32)
	content := "deadbeef  other.zip\n" + sum + "  WhatsAppDesk.exe\n"
	got, err := parseChecksumFile(content, "WhatsAppDesk.exe")
	if err != nil {
		t.Fatal(err)
	}
	if got != sum {
		t.Fatalf("got %q want %q", got, sum)
	}
}

func TestParseChecksumFileBSDStyle(t *testing.T) {
	sum := strings.Repeat("cd", 32)
	content := "SHA256 (WhatsApp-Desk-Linux-x64.tar.gz) = " + sum + "\n"
	got, err := parseChecksumFile(content, "WhatsApp-Desk-Linux-x64.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if got != sum {
		t.Fatalf("got %q want %q", got, sum)
	}
}

func TestParseChecksumFileHandlesBinaryMarkerAndPaths(t *testing.T) {
	sum := strings.Repeat("ef", 32)
	content := sum + " *release-assets/WhatsAppDesk.exe\n"
	got, err := parseChecksumFile(content, "WhatsAppDesk.exe")
	if err != nil {
		t.Fatal(err)
	}
	if got != sum {
		t.Fatalf("basename matching failed: %q", got)
	}
}

func TestParseChecksumFileRejectsMalformedDigest(t *testing.T) {
	if _, err := parseChecksumFile("nothex  WhatsAppDesk.exe\n", "WhatsAppDesk.exe"); err == nil {
		t.Fatal("expected malformed digest to be rejected")
	}
}

func TestParseChecksumFileReportsMissingAsset(t *testing.T) {
	_, err := parseChecksumFile(strings.Repeat("ab", 32)+"  other.exe\n", "WhatsAppDesk.exe")
	if err == nil || !strings.Contains(err.Error(), "not listed") {
		t.Fatalf("expected 'not listed' error, got %v", err)
	}
}

func TestVerifyDownloadedChecksumAcceptsMatch(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "WhatsAppDesk.exe")
	payload := []byte("fake binary payload")
	if err := os.WriteFile(file, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)

	withStubTransport(t, hex.EncodeToString(sum[:])+"  WhatsAppDesk.exe\n", 0)

	if err := verifyDownloadedChecksum(file, "WhatsAppDesk.exe", "https://example.invalid/dl", true); err != nil {
		t.Fatalf("matching checksum must pass: %v", err)
	}
}

func TestVerifyDownloadedChecksumRejectsMismatch(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "WhatsAppDesk.exe")
	if err := os.WriteFile(file, []byte("tampered payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	other := sha256.Sum256([]byte("the real payload"))

	withStubTransport(t, hex.EncodeToString(other[:])+"  WhatsAppDesk.exe\n", 0)

	// A mismatch must be fatal even when checksums are not yet required.
	for _, required := range []bool{true, false} {
		err := verifyDownloadedChecksum(file, "WhatsAppDesk.exe", "https://example.invalid/dl", required)
		if err == nil {
			t.Fatalf("tampered download must be rejected (required=%v)", required)
		}
		if !strings.Contains(err.Error(), "integrity check failed") {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestVerifyDownloadedChecksumMissingSumsRespectsPolicy(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "WhatsAppDesk.exe")
	if err := os.WriteFile(file, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}

	withStubTransport(t, "", http.StatusNotFound) // release predates SHA256SUMS

	if err := verifyDownloadedChecksum(file, "WhatsAppDesk.exe", "https://example.invalid/dl", false); err != nil {
		t.Fatalf("legacy release without sums must still update: %v", err)
	}
	if err := verifyDownloadedChecksum(file, "WhatsAppDesk.exe", "https://example.invalid/dl", true); err == nil {
		t.Fatal("required checksum with missing sums must fail")
	}
}

func TestReleaseBaseURLForAsset(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://github.com/vianziro/Whatsapp-Dekstop/releases/latest/download/WhatsAppDesk.exe",
			"https://github.com/vianziro/Whatsapp-Dekstop/releases/latest/download/"},
		{"https://github.com/vianziro/Whatsapp-Dekstop/releases/download/v1.5.9.2/WhatsApp-Desk-macOS-Universal.zip",
			"https://github.com/vianziro/Whatsapp-Dekstop/releases/download/v1.5.9.2/"},
		{"https://example.com/dl/WhatsApp-Desk-Linux-x64.tar.gz?token=abc",
			"https://example.com/dl/"},
	}
	for _, c := range cases {
		if got := releaseBaseURLForAsset(c.in); got != c.want {
			t.Errorf("releaseBaseURLForAsset(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestChecksumIsReadFromTheSameReleaseAsTheArtifact(t *testing.T) {
	// The checksum request must target the artifact's own release directory,
	// never a hardcoded "latest" that could describe a different build.
	dir := t.TempDir()
	file := filepath.Join(dir, "WhatsAppDesk.exe")
	payload := []byte("pinned-release binary")
	if err := os.WriteFile(file, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)

	var requested string
	prev := checksumHTTPTransport
	checksumHTTPTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requested = req.URL.String()
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(hex.EncodeToString(sum[:]) + "  WhatsAppDesk.exe\n")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	t.Cleanup(func() { checksumHTTPTransport = prev })

	artifact := "https://github.com/vianziro/Whatsapp-Dekstop/releases/download/v1.5.9.2/WhatsAppDesk.exe"
	if err := verifyDownloadedChecksum(file, "WhatsAppDesk.exe", releaseBaseURLForAsset(artifact), true); err != nil {
		t.Fatalf("pinned release checksum must verify: %v", err)
	}
	want := "https://github.com/vianziro/Whatsapp-Dekstop/releases/download/v1.5.9.2/" + checksumFileName
	if requested != want {
		t.Fatalf("checksum fetched from %q, want %q", requested, want)
	}
}

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
