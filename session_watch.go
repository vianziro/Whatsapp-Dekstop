package main

// Session watch: an always-on, local record of when an account's WhatsApp
// session appears, disappears, or the page is loaded again.
//
// "I scanned the QR code and was thrown out again later" used to leave no
// trace at all: wa_debug.log is opt-in (see debugEnabled), so on a normal
// install nothing was written when a signed-in account fell back to the QR
// page. This log is deliberately separate from the debug log so it can stay on
// for everyone: it is written only when the state CHANGES, holds no chat
// content, and is capped. Nothing is uploaded.

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// sessionLogMaxBytes caps wa_session.log; one previous generation is kept.
const sessionLogMaxBytes = 256 << 10

const (
	sessionStateSignedIn  = "signed-in"
	sessionStateSignedOut = "signed-out"
	sessionStatePageLoad  = "page-load"
)

func defaultSessionLogPath() string {
	return filepath.Join(filepath.Dir(debugLogPath()), "wa_session.log")
}

// sessionLogPath is a variable so tests can point it at a temp dir.
var sessionLogPath = defaultSessionLogPath

type sessionAccountState struct {
	state string
	since time.Time
}

var (
	sessionWatchMu    sync.Mutex
	sessionStates     = map[string]sessionAccountState{}
	sessionPageLoads  int
	sessionWatchClock = time.Now
)

// recordSessionState is bound to the page as reportSessionStateNative. state
// is one of the sessionState* constants; anything else is ignored. detail is a
// short, single-line hint that the page only fills in on the signed-out page
// (e.g. the text of WhatsApp's own error dialog). It reports whether a line
// was written.
func recordSessionState(state, detail string) bool {
	if state != sessionStateSignedIn && state != sessionStateSignedOut && state != sessionStatePageLoad {
		return false
	}
	// A test run must never append to the real profile's log.
	if testing.Testing() && sessionLogPath() == defaultSessionLogPath() {
		return false
	}
	accountID, accountLabel := activeAccountForSessionLog()

	sessionWatchMu.Lock()
	defer sessionWatchMu.Unlock()
	now := sessionWatchClock()

	if state == sessionStatePageLoad {
		sessionPageLoads++
		return appendSessionLogLocked(now, accountID, accountLabel,
			fmt.Sprintf("state=%s n=%d", sessionStatePageLoad, sessionPageLoads))
	}

	previous, known := sessionStates[accountID]
	if !known {
		// First report for this account in this process: the last line the
		// previous run wrote says whether the session was there before.
		if last := lastLoggedSessionStateLocked(accountID); last != "" {
			previous, known = sessionAccountState{state: last}, true
		}
	}
	if known && previous.state == state {
		if previous.since.IsZero() {
			// Same state as the previous run: remember it, write nothing.
			sessionStates[accountID] = sessionAccountState{state: state, since: now}
		}
		return false
	}
	sessionStates[accountID] = sessionAccountState{state: state, since: now}

	line := "state=" + state
	if known {
		line += " was=" + previous.state
		if !previous.since.IsZero() {
			line += " for=" + now.Sub(previous.since).Round(time.Second).String()
		} else {
			line += " for=previous-run"
		}
		if previous.state == sessionStateSignedIn && state == sessionStateSignedOut {
			line += " SESSION-LOST"
		}
	}
	if detail = sanitizeSessionDetail(detail); detail != "" && state == sessionStateSignedOut {
		line += fmt.Sprintf(" detail=%q", detail)
	}
	wrote := appendSessionLogLocked(now, accountID, accountLabel, line)
	cacheDebugLog("session: account=%q %s", accountLabel, line)
	return wrote
}

func activeAccountForSessionLog() (id, label string) {
	label = "?"
	registry, err := loadAccountRegistry()
	if err != nil {
		return "", label
	}
	for _, account := range registry.Accounts {
		if account.ID == registry.ActiveAccountID {
			label = account.Label
		}
	}
	return registry.ActiveAccountID, label
}

// noteSessionEvent records a rare event that explains a lost or misrouted
// session (started on the fallback profile, a missing data store, a proxy
// that could not be applied). These were written only to wa_debug.log, which
// is off unless the user opted in - so on a normal install the very warnings
// meant to make such a failure visible were never recorded anywhere.
func noteSessionEvent(format string, args ...any) {
	if testing.Testing() && sessionLogPath() == defaultSessionLogPath() {
		return
	}
	accountID, accountLabel := activeAccountForSessionLog()
	sessionWatchMu.Lock()
	defer sessionWatchMu.Unlock()
	appendSessionLogLocked(sessionWatchClock(), accountID, accountLabel,
		"notice: "+sanitizeSessionDetail(fmt.Sprintf(format, args...)))
}

func sanitizeSessionDetail(detail string) string {
	detail = strings.Join(strings.Fields(detail), " ")
	if len(detail) > 200 {
		detail = detail[:200]
	}
	return detail
}

// sessionLogAccountTag identifies an account in the log without spelling out
// its whole id; the label is the user's own name for it.
func sessionLogAccountTag(accountID, accountLabel string) string {
	short := accountID
	if len(short) > 8 {
		short = short[:8]
	}
	return fmt.Sprintf("account=%q id=%s", accountLabel, short)
}

func appendSessionLogLocked(now time.Time, accountID, accountLabel, line string) bool {
	path := sessionLogPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return false
	}
	if info, err := os.Stat(path); err == nil && info.Size() > sessionLogMaxBytes {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return false
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s %s %s\n", now.Format("2006-01-02 15:04:05"),
		sessionLogAccountTag(accountID, accountLabel), line)
	return err == nil
}

// lastLoggedSessionStateLocked returns the last signed-in/signed-out state
// logged for accountID by an earlier run, or "" when there is none.
func lastLoggedSessionStateLocked(accountID string) string {
	f, err := os.Open(sessionLogPath())
	if err != nil {
		return ""
	}
	defer f.Close()
	short := accountID
	if len(short) > 8 {
		short = short[:8]
	}
	marker := " id=" + short + " state="
	last := ""
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		text := scanner.Text()
		i := strings.Index(text, marker)
		if i < 0 {
			continue
		}
		state := text[i+len(marker):]
		if j := strings.IndexByte(state, ' '); j >= 0 {
			state = state[:j]
		}
		if state == sessionStateSignedIn || state == sessionStateSignedOut {
			last = state
		}
	}
	return last
}
