package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestInitScriptKeepsSettingsReachable guards the Windows regression where the
// Settings button and the Ctrl+, shortcut disappeared together: a single
// exception anywhere in the injected script aborted everything defined after
// it. The invariant is that no single failure may leave the user with no way to
// open Settings.
//
// The real init script is executed in a jsdom environment with a
// WhatsApp-Web-shaped DOM; failures are injected one at a time and the Settings
// entry point must survive each.
func TestInitScriptKeepsSettingsReachable(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not available")
	}

	// The harness needs jsdom, which may not be installed in every environment.
	harness := filepath.Join("testdata", "init_script_harness.js")
	if _, err := os.Stat(harness); err != nil {
		t.Skipf("harness missing: %v", err)
	}

	script := getInitScript("test-agent")
	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, "init_script.js")
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(node, harness, scriptPath)
	cmd.Env = append(os.Environ(), "NODE_PATH="+jsdomNodePath())
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") || strings.Contains(err.Error(), "permission denied") {
			t.Skipf("skipping node execution in sandboxed environment: %v", err)
		}
		var reason struct{ Skipped, Message string }
		_ = json.Unmarshal(out, &reason)
		if reason.Skipped != "" {
			t.Skip(reason.Skipped)
		}
		t.Fatalf("settings entry point is not failure-proof: %v\n%s", err, out)
	}
	t.Logf("harness output:\n%s", out)
}

// jsdomNodePath locates a node_modules directory that provides jsdom. The
// harness skips itself when none is found, so the suite stays green on hosts
// without a Node toolchain while still exercising the real script where jsdom
// is available. Override with WA_JSDOM_PATH.
func jsdomNodePath() string {
	if p := os.Getenv("WA_JSDOM_PATH"); p != "" {
		return p
	}
	candidates := []string{
		"node_modules",
		"/tmp/wa_harness/node_modules",
	}
	if out, err := exec.Command("npm", "root", "-g").Output(); err == nil {
		candidates = append(candidates, strings.TrimSpace(string(out)))
	}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(c, "jsdom")); err == nil {
			return c
		}
	}
	return candidates[0]
}
