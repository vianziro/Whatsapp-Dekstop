package main

import "testing"

func TestMacUserAgentSafariDiagnosticOverride(t *testing.T) {
	t.Setenv("WA_DESK_SAFARI_UA_TEST", "")
	if got := macUserAgent(); got != chromeUserAgent {
		t.Fatalf("default user agent = %q, want Chrome compatibility UA", got)
	}

	t.Setenv("WA_DESK_SAFARI_UA_TEST", "1")
	if got := macUserAgent(); got != safariTestUserAgent {
		t.Fatalf("diagnostic user agent = %q, want Safari test UA", got)
	}
}
