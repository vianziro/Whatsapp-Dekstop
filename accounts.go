package main

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const maxAccounts = 2

// Account contains only local display metadata. Session material remains owned
// by the native web engine in the account's isolated profile directory.
type Account struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	CreatedAt time.Time `json:"created_at"`
	// LastUnread is the badge value (e.g. "3", "999+") the account showed in
	// its title the last time it was switched away from. Only one engine is
	// live at a time, so this is a hint snapshot written at switch time — no
	// background polling, no timers. Empty means "no unread when last seen".
	LastUnread string `json:"last_unread,omitempty"`
}

type AccountRegistry struct {
	Version         int       `json:"version"`
	ActiveAccountID string    `json:"active_account_id"`
	Accounts        []Account `json:"accounts"`
}

var accountRegistryMu sync.Mutex

var accountRegistryFilePath = defaultAccountRegistryFilePath

func defaultAccountRegistryFilePath() string {
	return filepath.Join(filepath.Dir(getSettingsFilePath()), "accounts.json")
}

// legacyProfileDir deliberately keeps the original profile location intact.
// Default is therefore a migration by reference rather than a copy, which
// preserves the existing WhatsApp pairing and never handles session contents.
func legacyProfileDir() string {
	return filepath.Join(filepath.Dir(getSettingsFilePath()), "UserData")
}

func accountProfileDir(accountID string) (string, error) {
	registry, err := loadAccountRegistry()
	if err != nil {
		return "", err
	}
	if !registry.hasAccount(accountID) {
		return "", errors.New("account does not exist")
	}
	if accountID == registry.Accounts[0].ID {
		return legacyProfileDir(), nil
	}
	return filepath.Join(filepath.Dir(getSettingsFilePath()), "Profiles", accountID), nil
}

func defaultAccountRegistry() AccountRegistry {
	id := newAccountID()
	return AccountRegistry{
		Version:         1,
		ActiveAccountID: id,
		Accounts: []Account{{
			ID:        id,
			Label:     "Default",
			CreatedAt: time.Now().UTC(),
		}},
	}
}

// newAccountID returns a canonical, dashed UUID (8-4-4-4-12). This exact shape
// matters beyond cosmetics: macOS's NSUUID initWithUUIDString: silently
// returns nil for anything else, which previously made every account resolve
// to WebKit's shared default datastore instead of an isolated profile.
func newAccountID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failing is exceptional; fall back to a value that is still
		// unique and, deliberately, still a well-formed UUID shape.
		binary.BigEndian.PutUint64(b[:8], uint64(time.Now().UnixNano()))
		binary.BigEndian.PutUint64(b[8:], uint64(os.Getpid()))
	}
	// RFC 4122 version 4 / variant bits, so the value is a valid UUID even
	// though we don't need cryptographic randomness guarantees here.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func validateAccountLabel(label string) (string, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return "", errors.New("account label is required")
	}
	if len([]rune(label)) > 32 {
		return "", errors.New("account label must be 32 characters or fewer")
	}
	return label, nil
}

func (r AccountRegistry) hasAccount(id string) bool {
	for _, account := range r.Accounts {
		if account.ID == id {
			return true
		}
	}
	return false
}

func loadAccountRegistry() (AccountRegistry, error) {
	accountRegistryMu.Lock()
	defer accountRegistryMu.Unlock()

	// A missing registry file alongside an existing snapshot is not a fresh
	// install (fresh installs have neither): the registry file was lost. Start
	// from the snapshot instead of bootstrapping a brand-new default account,
	// which would orphan the second account's on-disk profile and drop that
	// user straight onto the QR page.
	if _, statErr := os.Stat(accountRegistryFilePath()); os.IsNotExist(statErr) {
		if snapshot, ok := loadAccountRegistrySnapshotLocked(); ok {
			cacheDebugLog("accounts registry file is missing; recovering the last known accounts snapshot")
			// Best effort: even if the heal fails, this run already has the
			// snapshot in hand.
			if healErr := saveAccountRegistryLocked(snapshot); healErr != nil {
				cacheDebugLog("accounts registry heal failed (%v); continuing from the snapshot", healErr)
			}
			return snapshot, nil
		}
	}

	var registry AccountRegistry
	var err error
	for attempt := 0; attempt < accountRegistryReadAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(accountRegistryReadRetryDelay)
		}
		registry, err = loadAccountRegistryLocked()
		if err == nil {
			// Migration: installs from before the snapshot existed hold a
			// valid registry but no snapshot yet. Seed it once so the very
			// first read failure is already covered.
			if _, ok := loadAccountRegistrySnapshotLocked(); !ok {
				saveAccountRegistrySnapshotLocked(registry)
			}
			return registry, nil
		}
	}
	// The registry is unreadable or corrupt after every attempt. Rather than
	// reporting an error up the chain - which the engine start loops used to
	// answer by silently loading the DEFAULT account's session, i.e. the other
	// account's WhatsApp - fall back to the last known good state. This is the
	// fix for "the two accounts suddenly alternate": a transient read failure
	// can no longer select the wrong profile.
	if snapshot, ok := loadAccountRegistrySnapshotLocked(); ok {
		cacheDebugLog("accounts registry unavailable (%v); using the last known accounts snapshot", err)
		// Heal the on-disk registry so the next read does not have to fall
		// back too. Best effort; the snapshot already covers this run.
		if healErr := saveAccountRegistryLocked(snapshot); healErr != nil {
			cacheDebugLog("accounts registry heal failed (%v); continuing from the snapshot", healErr)
		}
		return snapshot, nil
	}
	return AccountRegistry{}, err
}

const (
	accountRegistryReadAttempts   = 3
	accountRegistryReadRetryDelay = 50 * time.Millisecond
)

// accountSnapshotFilePath mirrors the registry location, so tests that
// relocate the registry automatically relocate the snapshot with it and cannot
// leak one test's snapshot into another's directory.
func accountSnapshotFilePath() string {
	return filepath.Join(filepath.Dir(accountRegistryFilePath()), "accounts.snapshot.json")
}

// loadAccountRegistrySnapshotLocked reads the last known good registry copy.
// Like the registry itself, a snapshot is only trusted when it is internally
// valid: right version, one or two accounts, and the active id among them.
func loadAccountRegistrySnapshotLocked() (AccountRegistry, bool) {
	data, err := os.ReadFile(accountSnapshotFilePath())
	if err != nil {
		return AccountRegistry{}, false
	}
	var registry AccountRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		return AccountRegistry{}, false
	}
	if registry.Version != 1 || len(registry.Accounts) == 0 || len(registry.Accounts) > maxAccounts || !registry.hasAccount(registry.ActiveAccountID) {
		return AccountRegistry{}, false
	}
	return registry, true
}

// saveAccountRegistrySnapshotLocked atomically copies the registry to the
// snapshot. Best effort by design: a failed snapshot write must never break
// the operation that produced the registry change.
func saveAccountRegistrySnapshotLocked(registry AccountRegistry) {
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return
	}
	dir := filepath.Dir(accountSnapshotFilePath())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, "accounts-snap-*.tmp")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return
	}
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}
	_ = os.Rename(tmpName, accountSnapshotFilePath())
}

func loadAccountRegistryLocked() (AccountRegistry, error) {
	data, err := os.ReadFile(accountRegistryFilePath())
	if os.IsNotExist(err) {
		registry := defaultAccountRegistry()
		return registry, saveAccountRegistryLocked(registry)
	}
	if err != nil {
		return AccountRegistry{}, err
	}
	var registry AccountRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		return AccountRegistry{}, fmt.Errorf("read accounts registry: %w", err)
	}
	if registry.Version != 1 || len(registry.Accounts) == 0 || len(registry.Accounts) > maxAccounts || !registry.hasAccount(registry.ActiveAccountID) {
		return AccountRegistry{}, errors.New("accounts registry is invalid")
	}
	return registry, nil
}

func saveAccountRegistry(registry AccountRegistry) error {
	accountRegistryMu.Lock()
	defer accountRegistryMu.Unlock()
	return saveAccountRegistryLocked(registry)
}

func saveAccountRegistryLocked(registry AccountRegistry) error {
	if registry.Version != 1 || len(registry.Accounts) == 0 || len(registry.Accounts) > maxAccounts || !registry.hasAccount(registry.ActiveAccountID) {
		return errors.New("refusing to save invalid accounts registry")
	}
	if err := os.MkdirAll(filepath.Dir(accountRegistryFilePath()), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(accountRegistryFilePath()), "accounts-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, accountRegistryFilePath()); err != nil {
		return err
	}
	// Every successful registry write also refreshes the last known good
	// snapshot, so the fallback in loadAccountRegistry always reflects the
	// newest state the app ever had.
	saveAccountRegistrySnapshotLocked(registry)
	return nil
}

func createAccount(label string) (Account, error) {
	label, err := validateAccountLabel(label)
	if err != nil {
		return Account{}, err
	}
	accountRegistryMu.Lock()
	defer accountRegistryMu.Unlock()
	registry, err := loadAccountRegistryLocked()
	if err != nil {
		return Account{}, err
	}
	if len(registry.Accounts) >= maxAccounts {
		return Account{}, fmt.Errorf("a maximum of %d accounts is supported", maxAccounts)
	}
	account := Account{ID: newAccountID(), Label: label, CreatedAt: time.Now().UTC()}
	registry.Accounts = append(registry.Accounts, account)
	if err := saveAccountRegistryLocked(registry); err != nil {
		return Account{}, err
	}
	return account, nil
}

func renameAccount(id, label string) (Account, error) {
	label, err := validateAccountLabel(label)
	if err != nil {
		return Account{}, err
	}
	accountRegistryMu.Lock()
	defer accountRegistryMu.Unlock()
	registry, err := loadAccountRegistryLocked()
	if err != nil {
		return Account{}, err
	}
	for i := range registry.Accounts {
		if registry.Accounts[i].ID == id {
			registry.Accounts[i].Label = label
			if err := saveAccountRegistryLocked(registry); err != nil {
				return Account{}, err
			}
			return registry.Accounts[i], nil
		}
	}
	return Account{}, errors.New("account does not exist")
}

// accountsForUI returns the minimum display information the injected page
// needs. The profile path is intentionally never exposed to JavaScript.
func accountsForUI() ([]map[string]any, error) {
	registry, err := loadAccountRegistry()
	if err != nil {
		return nil, err
	}
	accounts := make([]map[string]any, 0, len(registry.Accounts))
	for _, account := range registry.Accounts {
		accounts = append(accounts, map[string]any{
			"id":          account.ID,
			"label":       account.Label,
			"active":      account.ID == registry.ActiveAccountID,
			"last_unread": account.LastUnread,
		})
	}
	return accounts, nil
}

func activeAccountProfileIdentifier() (string, error) {
	registry, err := loadAccountRegistry()
	if err != nil {
		return "", err
	}
	// The legacy/default account intentionally remains on WebKit's original
	// default datastore. Every additional account receives an isolated store.
	if registry.ActiveAccountID == registry.Accounts[0].ID {
		return "", nil
	}
	return registry.ActiveAccountID, nil
}

func isActiveAccount(id string) bool {
	registry, err := loadAccountRegistry()
	return err == nil && registry.ActiveAccountID == id
}

// activeAccountDataPath returns the on-disk profile directory for the active
// account, creating it when it is new. The first account deliberately returns
// the legacy profile location (by reference, never a copy), so an existing
// pairing keeps working; see legacyProfileDir. Engines whose native isolation
// is path-based — WebView2 user-data folders and the WebKitGTK website data
// manager — use this as their per-account storage root.
func activeAccountDataPath() (string, error) {
	id, err := activeAccountProfileIdentifier()
	if err != nil {
		return "", err
	}
	if id == "" {
		return legacyProfileDir(), nil
	}
	dir, err := accountProfileDir(id)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

func setActiveAccount(id string) error {
	accountRegistryMu.Lock()
	defer accountRegistryMu.Unlock()
	registry, err := loadAccountRegistryLocked()
	if err != nil {
		return err
	}
	if !registry.hasAccount(id) {
		return errors.New("account does not exist")
	}
	registry.ActiveAccountID = id
	return saveAccountRegistryLocked(registry)
}

// sanitizeLastUnread keeps the badge hint tiny and harmless: only digits and
// a plus sign, at most 6 characters, and "0" collapses to empty. The value
// originates in the page's <title>, so it is never trusted verbatim.
func sanitizeLastUnread(value string) string {
	var kept []rune
	for _, r := range value {
		if (r >= '0' && r <= '9') || r == '+' {
			kept = append(kept, r)
			if len(kept) == 6 {
				break
			}
		}
	}
	cleaned := string(kept)
	if cleaned == "0" || cleaned == "+0" {
		return ""
	}
	return cleaned
}

// setLastUnreadForActiveAccount records the outgoing account's badge hint at
// switch time. Called from the switch bridge only; no recurring writes.
func setLastUnreadForActiveAccount(value string) error {
	value = sanitizeLastUnread(value)
	accountRegistryMu.Lock()
	defer accountRegistryMu.Unlock()
	registry, err := loadAccountRegistryLocked()
	if err != nil {
		return err
	}
	for i := range registry.Accounts {
		if registry.Accounts[i].ID == registry.ActiveAccountID {
			if registry.Accounts[i].LastUnread == value {
				return nil
			}
			registry.Accounts[i].LastUnread = value
			return saveAccountRegistryLocked(registry)
		}
	}
	return errors.New("account does not exist")
}
