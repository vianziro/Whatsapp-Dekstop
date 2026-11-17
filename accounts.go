package main

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

// accountProfilesRoot holds the isolated profile of every account but the
// first. It sits beside accounts.json (which itself sits beside the settings
// file in production), so tests that relocate the registry also relocate every
// profile directory and never touch the real user profile.
func accountProfilesRoot() string {
	return filepath.Join(filepath.Dir(accountRegistryFilePath()), "Profiles")
}

func accountProfileDir(accountID string) (string, error) {
	registry, err := loadAccountRegistry()
	if err != nil {
		return "", err
	}
	return registry.profileDirFor(accountID)
}

// profileDirFor resolves an account's profile directory from a registry the
// caller already holds, so hot paths (engine start, every switch) read
// accounts.json once instead of once per lookup.
func (r AccountRegistry) profileDirFor(accountID string) (string, error) {
	if !r.hasAccount(accountID) {
		return "", errors.New("account does not exist")
	}
	if accountID == r.Accounts[0].ID {
		return legacyProfileDir(), nil
	}
	return filepath.Join(accountProfilesRoot(), accountID), nil
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
	return loadAccountRegistryResilientLocked()
}

// loadAccountRegistryResilientLocked is the one read path for the registry:
// retries, snapshot recovery for a corrupt or lost file, and the snapshot
// heal. Every mutation (create, rename, switch, remove) goes through it too.
// Mutations used to call the raw reader, which answered a lost accounts.json
// by minting a brand-new default registry and SAVING it - so a single missed
// read during an account switch permanently dropped the second account and
// overwrote the snapshot that could have recovered it.
func loadAccountRegistryResilientLocked() (AccountRegistry, error) {
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
			// first read failure is already covered. Checked once per process
			// (per snapshot location) rather than re-reading the snapshot on
			// every registry read: the dock and engine loop read often.
			if accountSnapshotSeededFor != accountSnapshotFilePath() {
				if _, ok := loadAccountRegistrySnapshotLocked(); !ok {
					saveAccountRegistrySnapshotLocked(registry)
				}
				accountSnapshotSeededFor = accountSnapshotFilePath()
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

// accountSnapshotSeededFor remembers the snapshot path already verified this
// process. Guarded by accountRegistryMu. Keyed by path so tests that relocate
// the registry each get their own first-load seeding.
var accountSnapshotSeededFor string

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
	registry, err := loadAccountRegistryResilientLocked()
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
	registry, err := loadAccountRegistryResilientLocked()
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

// activeAccountDataPath returns the on-disk profile directory for the active
// account, creating it when it is new. The first account deliberately returns
// the legacy profile location (by reference, never a copy), so an existing
// pairing keeps working; see legacyProfileDir. Engines whose native isolation
// is path-based — WebView2 user-data folders and the WebKitGTK website data
// manager — use this as their per-account storage root.
func activeAccountDataPath() (string, error) {
	// One registry read for the whole resolution; this runs on every engine
	// (re)build, i.e. on every account switch on Windows and Linux.
	registry, err := loadAccountRegistry()
	if err != nil {
		return "", err
	}
	dir, err := registry.profileDirFor(registry.ActiveAccountID)
	if err != nil {
		return "", err
	}
	if dir == legacyProfileDir() {
		return dir, nil
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

// switchActiveAccount records the outgoing account's unread hint and makes
// id the active account, as ONE locked read-modify-write. The bridges used to
// do this as three separate registry round-trips (isActiveAccount, then
// setLastUnreadForActiveAccount, then setActiveAccount): three reads, two
// writes, and a window between them where another write could interleave.
// changed is false - with no error and no write - when id is already active.
func switchActiveAccount(id, outgoingLastUnread string) (changed bool, err error) {
	accountRegistryMu.Lock()
	defer accountRegistryMu.Unlock()
	registry, err := loadAccountRegistryResilientLocked()
	if err != nil {
		return false, err
	}
	if !registry.hasAccount(id) {
		return false, errors.New("account does not exist")
	}
	if registry.ActiveAccountID == id {
		return false, nil
	}
	hint := sanitizeLastUnread(outgoingLastUnread)
	for i := range registry.Accounts {
		if registry.Accounts[i].ID == registry.ActiveAccountID {
			registry.Accounts[i].LastUnread = hint
		}
	}
	registry.ActiveAccountID = id
	if err := saveAccountRegistryLocked(registry); err != nil {
		return false, err
	}
	return true, nil
}

// errFirstAccountNotRemovable and errActiveAccountNotRemovable are surfaced to
// the dock verbatim, so they are phrased for the user.
var (
	errFirstAccountNotRemovable  = errors.New("The first account can't be removed. To use another number there, log out inside WhatsApp (Menu → Log out).")
	errActiveAccountNotRemovable = errors.New("Switch to the other account first, then remove this one.")
)

// removeAccount drops a saved account from the registry and retires its
// profile data. Two accounts are never removable:
//   - the first one, because it IS the legacy profile (adopted by reference,
//     see legacyProfileDir); retiring it would also retire the pre-multi-
//     account session data and leave the registry with no default slot;
//   - the active one, because its engine owns the profile right now
//     (WebView2 forbids touching a user-data folder in use, and WebKit's
//     network process holds the store open).
//
// Profile data is moved aside rather than deleted, matching the design doc's
// "recoverably removed": nothing is destroyed if the user removed the wrong
// account, and the next "Add account" always starts from a clean profile
// because it gets a fresh id.
func removeAccount(id string) (Account, error) {
	accountRegistryMu.Lock()
	registry, err := loadAccountRegistryResilientLocked()
	if err != nil {
		accountRegistryMu.Unlock()
		return Account{}, err
	}
	if !registry.hasAccount(id) {
		accountRegistryMu.Unlock()
		return Account{}, errors.New("account does not exist")
	}
	if registry.Accounts[0].ID == id {
		accountRegistryMu.Unlock()
		return Account{}, errFirstAccountNotRemovable
	}
	if registry.ActiveAccountID == id {
		accountRegistryMu.Unlock()
		return Account{}, errActiveAccountNotRemovable
	}
	var removed Account
	kept := make([]Account, 0, len(registry.Accounts))
	for _, account := range registry.Accounts {
		if account.ID == id {
			removed = account
			continue
		}
		kept = append(kept, account)
	}
	registry.Accounts = kept
	if err := saveAccountRegistryLocked(registry); err != nil {
		accountRegistryMu.Unlock()
		return Account{}, err
	}
	accountRegistryMu.Unlock()

	// Outside the lock: renames can be slow on network/AV-scanned drives and
	// the registry is already consistent without them.
	retireAccountProfileData(id)
	return removed, nil
}

// accountWebKitStoreRoot is where macOS WebKit keeps each non-default
// account's persistent website data store (see auditAccountWebKitStore). A
// variable so tests can point it at a temp dir.
var accountWebKitStoreRoot = func() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "WebKit", "com.whatsapp.desk", "WebsiteDataStore")
}

// accountProfileDataDirs lists every on-disk location an account's session
// may live in on this platform.
func accountProfileDataDirs(id string) []string {
	dirs := []string{filepath.Join(accountProfilesRoot(), id)}
	if runtime.GOOS == "darwin" {
		if root := accountWebKitStoreRoot(); root != "" {
			dirs = append(dirs, filepath.Join(root, webKitStoreDirName(id)))
		}
	}
	return dirs
}

// webKitStoreDirName is the folder WebKit names an account's data store. The
// engine (webview.h) turns the account id into an NSUUID, and WebKit names the
// folder after that UUID in its canonical dashed, lowercase form. Accounts
// created before newAccountID emitted dashes are stored as 32 undashed hex
// characters, so joining the raw id pointed at a folder that never exists:
// removing such an account left its logged-in session where it was.
func webKitStoreDirName(id string) string {
	if len(id) != 32 {
		return strings.ToLower(id)
	}
	for _, r := range id {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F') {
			return id
		}
	}
	id = strings.ToLower(id)
	return id[0:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:32]
}

// isSafeAccountPathElement guards the only destructive filesystem operation
// keyed by registry content. accounts.json is a user-writable file, so an id
// such as "../.." must never be joined into a path that is then renamed.
func isSafeAccountPathElement(id string) bool {
	return id != "" && id != "." && id != ".." && !strings.ContainsAny(id, `/\:`) && filepath.Base(id) == id
}

// retireAccountProfileData moves a removed account's profile aside as
// "<dir>.removed-<unix>". Best effort: a failure (e.g. a WebView2 helper
// process that has not exited yet still holding the folder) is logged and
// leaves the data where it was; the account is already gone from the
// registry, so the leftover folder is simply never loaded again.
var retireAccountProfileData = func(id string) {
	if !isSafeAccountPathElement(id) {
		cacheDebugLog("account remove: refusing to touch profile data for unsafe id %q", id)
		return
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	for _, dir := range accountProfileDataDirs(id) {
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		target := dir + ".removed-" + stamp
		if err := os.Rename(dir, target); err != nil {
			cacheDebugLog("account remove: could not move %s aside (%v); it is no longer used", dir, err)
			continue
		}
		cacheDebugLog("account remove: moved %s to %s", dir, target)
	}
}

func setActiveAccount(id string) error {
	accountRegistryMu.Lock()
	defer accountRegistryMu.Unlock()
	registry, err := loadAccountRegistryResilientLocked()
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
	registry, err := loadAccountRegistryResilientLocked()
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

// labelNotificationTitle prefixes a native notification with the active
// account's label once a second account exists ("Work · Alice"), so a
// notification says which WhatsApp it came from. With a single account the
// title is untouched. Privacy Mode redaction happens in the page before the
// bridge is called, so only the user-chosen label is ever added here.
func labelNotificationTitle(title string) string {
	// App-level notices (the updater's) are not from either account.
	if title == appUpdateNotificationTitle {
		return title
	}
	registry, err := loadAccountRegistry()
	if err != nil || len(registry.Accounts) < 2 {
		return title
	}
	for _, account := range registry.Accounts {
		if account.ID != registry.ActiveAccountID {
			continue
		}
		if strings.TrimSpace(title) == "" {
			return account.Label
		}
		return account.Label + " · " + title
	}
	return title
}

// appUpdateNotificationTitle mirrors the title the page uses for the update
// notice (main.go, "var notifTitle = 'Update Available'").
const appUpdateNotificationTitle = "Update Available"
