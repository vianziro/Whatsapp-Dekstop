package main

import (
	"strings"
	"testing"
)

// "Message a phone number" (#89) is page script plus a macOS menu item; the
// suite cannot drive the page, so its wiring is pinned by inspection.
// normalizeChatPhoneNumber itself was exercised with node against the
// injected script (international, 00-prefixed, local 0-prefixed, too short,
// too long and non-digit input).
func TestMessageAPhoneNumberIsWired(t *testing.T) {
	script := getInitScript("UA")
	for _, want := range []string{
		"function normalizeChatPhoneNumber(raw)",
		"window.location.href = 'https://web.whatsapp.com/send?phone=' + encodeURIComponent(digits);",
		"window.openChatByNumber = openChatByNumber;",
		"id: 'chatnumber'",
		"(e.key !== 'o' && e.key !== 'O')",
		"if (name === 'chatnumber')",
		"+Shift+O</strong> &mdash; Message a phone number",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("init script is missing %q", want)
		}
	}
	// Cmd/Ctrl+Shift+O must belong to this feature alone.
	if n := strings.Count(script, "e.key !== 'o'") + strings.Count(script, "e.key === 'o'"); n != 1 {
		t.Errorf("Cmd/Ctrl+Shift+O is handled %d times in the page script, want 1", n)
	}
	// Only a full page load reaches /send, so it must not run mid-switch.
	open := script[strings.Index(script, "function openChatByNumber()"):]
	if !strings.Contains(open[:200], "if (switchingId !== null) return;") {
		t.Error("openChatByNumber must refuse while an account switch is in progress")
	}

	darwin := readProjectFile(t, "app_darwin.go")
	for _, want := range []string{"- (void)menuMessageNumber:(id)sender;", "@selector(menuMessageNumber:)", "window.openChatByNumber()"} {
		if !strings.Contains(darwin, want) {
			t.Errorf("app_darwin.go is missing %q", want)
		}
	}
	if !strings.Contains(readProjectFile(t, "onboarding.go"), "['Message a phone number', mod + ' + Shift + O']") {
		t.Error("the onboarding shortcut list does not mention Message a phone number")
	}
}
