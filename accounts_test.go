package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

var canonicalUUIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func withAccountRegistryFile(t *testing.T) {
	t.Helper()
	original := accountRegistryFilePath
	path := filepath.Join(t.TempDir(), "accounts.json")
	accountRegistryFilePath = func() string { return path }
	t.Cleanup(func() { accountRegistryFilePath = original })
}

func TestAccountRegistryBootstrapsDefaultWithoutProfileMigration(t *testing.T) {
	withAccountRegistryFile(t)

	registry, err := loadAccountRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if registry.Version != 1 || len(registry.Accounts) != 1 {
		t.Fatalf("unexpected bootstrap registry: %#v", registry)
	}
	if registry.ActiveAccountID != registry.Accounts[0].ID || registry.Accounts[0].Label != "Default" {
		t.Fatalf("legacy session must be represented by active Default account: %#v", registry)
	}
}

func TestAccountRegistryAllowsAtMostTwoAccounts(t *testing.T) {
	withAccountRegistryFile(t)
	if _, err := loadAccountRegistry(); err != nil {
		t.Fatal(err)
	}
	work, err := createAccount("Work")
	if err != nil {
		t.Fatal(err)
	}
	if work.Label != "Work" || work.ID == "" {
		t.Fatalf("invalid created account: %#v", work)
	}
	if _, err := createAccount("Third"); err == nil {
		t.Fatal("third account must be rejected")
	}
}

func TestSanitizeLastUnread(t *testing.T) {
	cases := map[string]string{
		"3":        "3",
		"999+":     "999+",
		"(12)":     "12",
		"":         "",
		"0":        "",
		"hack;<i>": "",
		"12345678": "123456",
		"7\n":      "7",
	}
	for input, want := range cases {
		if got := sanitizeLastUnread(input); got != want {
			t.Errorf("sanitizeLastUnread(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSetLastUnreadForActiveAccountRoundTrip(t *testing.T) {
	withAccountRegistryFile(t)
	if _, err := loadAccountRegistry(); err != nil {
		t.Fatal(err)
	}
	work, err := createAccount("Work")
	if err != nil {
		t.Fatal(err)
	}
	if err := setActiveAccount(work.ID); err != nil {
		t.Fatal(err)
	}
	if err := setLastUnreadForActiveAccount("5"); err != nil {
		t.Fatal(err)
	}
	accounts, err := accountsForUI()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]map[string]any{}
	for _, a := range accounts {
		byID[a["id"].(string)] = a
	}
	if got := byID[work.ID]["last_unread"]; got != "5" {
		t.Fatalf("active account hint = %v, want 5", got)
	}
	// "0" collapses to empty: an unread-free account must not show a hint.
	if err := setLastUnreadForActiveAccount("0"); err != nil {
		t.Fatal(err)
	}
	accounts, _ = accountsForUI()
	for _, a := range accounts {
		if a["id"].(string) == work.ID && a["last_unread"] != "" {
			t.Fatalf("hint should collapse to empty, got %v", a["last_unread"])
		}
	}
}

func TestActiveAccountDataPathAdoptsLegacyProfileByReference(t *testing.T) {
	withAccountRegistryFile(t)

	got, err := activeAccountDataPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != legacyProfileDir() {
		t.Fatalf("first account must resolve to the legacy profile dir %q, got %q", legacyProfileDir(), got)
	}
}

func TestActiveAccountDataPathIsolatesSecondAccount(t *testing.T) {
	withAccountRegistryFile(t)
	if _, err := loadAccountRegistry(); err != nil {
		t.Fatal(err)
	}
	second, err := createAccount("Work")
	if err != nil {
		t.Fatal(err)
	}
	if err := setActiveAccount(second.ID); err != nil {
		t.Fatal(err)
	}

	got, err := activeAccountDataPath()
	if err != nil {
		t.Fatal(err)
	}
	if got == legacyProfileDir() {
		t.Fatal("second account must never use the legacy profile dir")
	}
	if filepath.Base(got) != second.ID {
		t.Fatalf("second account profile dir must be named by its account id, got %q", got)
	}
}

func TestAccountRegistryPersistsActiveAccountAndRename(t *testing.T) {
	withAccountRegistryFile(t)
	initial, err := loadAccountRegistry()
	if err != nil {
		t.Fatal(err)
	}
	work, err := createAccount("Work")
	if err != nil {
		t.Fatal(err)
	}
	if err := setActiveAccount(work.ID); err != nil {
		t.Fatal(err)
	}
	renamed, err := renameAccount(work.ID, "Client Support")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Label != "Client Support" {
		t.Fatalf("rename result = %q", renamed.Label)
	}
	registry, err := loadAccountRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if registry.ActiveAccountID != work.ID || registry.Accounts[0].ID != initial.Accounts[0].ID {
		t.Fatalf("unexpected persisted registry: %#v", registry)
	}
}

func TestNewAccountIDIsACanonicalUUID(t *testing.T) {
	// macOS's NSUUID initWithUUIDString: silently returns nil for anything that
	// isn't the dashed 8-4-4-4-12 shape, which used to make every account
	// silently fall back to WebKit's shared default datastore.
	for i := 0; i < 20; i++ {
		id := newAccountID()
		if !canonicalUUIDPattern.MatchString(id) {
			t.Fatalf("newAccountID produced a non-canonical UUID: %q", id)
		}
	}
}

func TestAccountRegistryRejectsInvalidLabelsAndUnknownAccount(t *testing.T) {
	withAccountRegistryFile(t)
	if _, err := loadAccountRegistry(); err != nil {
		t.Fatal(err)
	}
	if _, err := createAccount("   "); err == nil {
		t.Fatal("blank label must be rejected")
	}
	if err := setActiveAccount("missing"); err == nil {
		t.Fatal("unknown account must be rejected")
	}
}

// seedSecondAccountActive builds the two-account state the fallback tests
// need: "Work" exists, is active, and a snapshot of that state is on disk.
func seedSecondAccountActive(t *testing.T) (workID string, registryPath string) {
	t.Helper()
	if _, err := loadAccountRegistry(); err != nil {
		t.Fatal(err)
	}
	work, err := createAccount("Work")
	if err != nil {
		t.Fatal(err)
	}
	if err := setActiveAccount(work.ID); err != nil {
		t.Fatal(err)
	}
	return work.ID, accountRegistryFilePath()
}

func TestAccountRegistrySnapshotSeededOnFirstLoad(t *testing.T) {
	withAccountRegistryFile(t)

	registry, err := loadAccountRegistry()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(accountSnapshotFilePath())
	if err != nil {
		t.Fatalf("first successful load must seed the snapshot: %v", err)
	}
	var snapshot AccountRegistry
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatalf("snapshot is not a valid registry copy: %v", err)
	}
	if snapshot.ActiveAccountID != registry.ActiveAccountID || len(snapshot.Accounts) != len(registry.Accounts) {
		t.Fatalf("snapshot %#v does not match registry %#v", snapshot, registry)
	}
}

func TestAccountRegistrySnapshotRecoversFromCorruptRegistry(t *testing.T) {
	withAccountRegistryFile(t)
	workID, registryPath := seedSecondAccountActive(t)

	// Simulate the failure that used to flip the app onto the other account's
	// session: an unreadable registry at engine (re)build time.
	if err := os.WriteFile(registryPath, []byte("{corrupted"), 0600); err != nil {
		t.Fatal(err)
	}

	registry, err := loadAccountRegistry()
	if err != nil {
		t.Fatalf("a corrupt registry must fall back to the snapshot, got error: %v", err)
	}
	if registry.ActiveAccountID != workID {
		t.Fatalf("fallback must keep the last known active account %q, got %q", workID, registry.ActiveAccountID)
	}
	if len(registry.Accounts) != 2 {
		t.Fatalf("fallback must keep both accounts, got %#v", registry.Accounts)
	}
	// The fallback also heals the on-disk registry, so the next read is clean.
	again, err := loadAccountRegistry()
	if err != nil {
		t.Fatalf("healed registry must read normally: %v", err)
	}
	if again.ActiveAccountID != workID {
		t.Fatalf("healed registry lost the active account: %#v", again)
	}
}

func TestAccountRegistrySnapshotRecoversFromMissingRegistryFile(t *testing.T) {
	withAccountRegistryFile(t)
	workID, registryPath := seedSecondAccountActive(t)

	if err := os.Remove(registryPath); err != nil {
		t.Fatal(err)
	}

	registry, err := loadAccountRegistry()
	if err != nil {
		t.Fatalf("a lost registry file must fall back to the snapshot, got error: %v", err)
	}
	if registry.ActiveAccountID != workID {
		t.Fatalf("recovery must keep the last known active account %q, got %q", workID, registry.ActiveAccountID)
	}
	// A brand-new bootstrap would have minted a fresh default account and
	// orphaned the second account's profile; that must not happen.
	if len(registry.Accounts) != 2 {
		t.Fatalf("recovery must keep both accounts, got %#v", registry.Accounts)
	}
	if _, err := os.Stat(registryPath); err != nil {
		t.Fatalf("recovery must heal the registry file back onto disk: %v", err)
	}
}

// --- mutations survive a lost or corrupt registry ---------------------------

// TestAccountMutationsRecoverFromLostRegistryInsteadOfMintingADefault pins the
// switch-time data-loss bug: setActiveAccount / switchActiveAccount used the
// raw reader, which answers a missing accounts.json by minting a NEW default
// registry and saving it - dropping the second account and overwriting the
// very snapshot that could have recovered it.
func TestAccountMutationsRecoverFromLostRegistryInsteadOfMintingADefault(t *testing.T) {
	withAccountRegistryFile(t)
	workID, registryPath := seedSecondAccountActive(t)
	before, err := loadAccountRegistry()
	if err != nil {
		t.Fatal(err)
	}
	defaultID := before.Accounts[0].ID

	if err := os.Remove(registryPath); err != nil {
		t.Fatal(err)
	}
	changed, err := switchActiveAccount(defaultID, "4")
	if err != nil || !changed {
		t.Fatalf("switch after a lost registry must recover and succeed, got changed=%v err=%v", changed, err)
	}
	after, err := loadAccountRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Accounts) != 2 || !after.hasAccount(workID) || after.Accounts[0].ID != defaultID {
		t.Fatalf("a lost registry must not cost the second account or re-key the default one: %#v", after.Accounts)
	}
	if after.ActiveAccountID != defaultID {
		t.Fatalf("switch did not apply: active=%q", after.ActiveAccountID)
	}

	if err := os.WriteFile(registryPath, []byte("{corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := renameAccount(workID, "Office"); err != nil {
		t.Fatalf("rename must recover from a corrupt registry via the snapshot: %v", err)
	}
	if _, err := createAccount("Third"); err == nil {
		t.Fatal("recovered registry must still enforce the two-account cap")
	}
}

func TestSwitchActiveAccountRecordsOutgoingHintAtomically(t *testing.T) {
	withAccountRegistryFile(t)
	registry, err := loadAccountRegistry()
	if err != nil {
		t.Fatal(err)
	}
	defaultID := registry.Accounts[0].ID
	work, err := createAccount("Work")
	if err != nil {
		t.Fatal(err)
	}

	changed, err := switchActiveAccount(work.ID, "(7)")
	if err != nil || !changed {
		t.Fatalf("switch to Work failed: changed=%v err=%v", changed, err)
	}
	registry, _ = loadAccountRegistry()
	if registry.ActiveAccountID != work.ID {
		t.Fatalf("active account not flipped: %#v", registry)
	}
	if registry.Accounts[0].LastUnread != "7" {
		t.Fatalf("outgoing account's unread hint must be recorded (sanitized), got %q", registry.Accounts[0].LastUnread)
	}

	changed, err = switchActiveAccount(work.ID, "9")
	if err != nil || changed {
		t.Fatalf("switching to the already-active account must be a no-op, got changed=%v err=%v", changed, err)
	}
	registry, _ = loadAccountRegistry()
	if registry.Accounts[1].LastUnread != "" {
		t.Fatalf("a no-op switch must not write a hint, got %q", registry.Accounts[1].LastUnread)
	}
	if _, err := switchActiveAccount("missing", ""); err == nil {
		t.Fatal("switch to an unknown account must fail")
	}
	if changed, _ := switchActiveAccount(defaultID, ""); !changed {
		t.Fatal("switch back to the default account must apply")
	}
}

// --- removing an account -----------------------------------------------------

func withAccountWebKitStoreRoot(t *testing.T) string {
	t.Helper()
	original := accountWebKitStoreRoot
	root := filepath.Join(t.TempDir(), "WebsiteDataStore")
	accountWebKitStoreRoot = func() string { return root }
	t.Cleanup(func() { accountWebKitStoreRoot = original })
	return root
}

func TestRemoveAccountRefusesFirstAndActiveAccounts(t *testing.T) {
	withAccountRegistryFile(t)
	withAccountWebKitStoreRoot(t)
	workID, _ := seedSecondAccountActive(t)
	registry, _ := loadAccountRegistry()
	defaultID := registry.Accounts[0].ID

	if _, err := removeAccount(defaultID); err != errFirstAccountNotRemovable {
		t.Fatalf("the first (legacy-profile) account must not be removable, got %v", err)
	}
	if _, err := removeAccount(workID); err != errActiveAccountNotRemovable {
		t.Fatalf("the active account must not be removable while its engine owns the profile, got %v", err)
	}
	if _, err := removeAccount("missing"); err == nil {
		t.Fatal("removing an unknown account must fail")
	}
	after, _ := loadAccountRegistry()
	if len(after.Accounts) != 2 {
		t.Fatalf("refused removals must not change the registry: %#v", after.Accounts)
	}
}

// Accounts created before ids were dashed are stored as 32 hex characters,
// while WebKit names the store folder after the dashed UUID. Retiring such an
// account must still find its store.
func TestWebKitStoreDirNameCanonicalizesLegacyIDs(t *testing.T) {
	cases := map[string]string{
		"f7d90df2753c333d134b5696cb7d1fe6":     "f7d90df2-753c-333d-134b-5696cb7d1fe6",
		"F7D90DF2753C333D134B5696CB7D1FE6":     "f7d90df2-753c-333d-134b-5696cb7d1fe6",
		"f7d90df2-753c-333d-134b-5696cb7d1fe6": "f7d90df2-753c-333d-134b-5696cb7d1fe6",
		"zzd90df2753c333d134b5696cb7d1fe6":     "zzd90df2753c333d134b5696cb7d1fe6",
	}
	for id, want := range cases {
		if got := webKitStoreDirName(id); got != want {
			t.Errorf("webKitStoreDirName(%q) = %q, want %q", id, got, want)
		}
	}
	if id := newAccountID(); webKitStoreDirName(id) != id {
		t.Errorf("a freshly minted id must already be the store folder name: %q", id)
	}
}

func TestRetireAccountProfileDataFindsLegacyWebKitStore(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("WebKit data stores exist only on macOS")
	}
	withAccountRegistryFile(t)
	storeRoot := withAccountWebKitStoreRoot(t)
	const legacyID = "f7d90df2753c333d134b5696cb7d1fe6"
	store := filepath.Join(storeRoot, "f7d90df2-753c-333d-134b-5696cb7d1fe6")
	if err := os.MkdirAll(store, 0700); err != nil {
		t.Fatal(err)
	}

	retireAccountProfileData(legacyID)

	if _, err := os.Stat(store); !os.IsNotExist(err) {
		t.Fatalf("the legacy account's WebKit store must be moved aside, stat err = %v", err)
	}
	matches, _ := filepath.Glob(store + ".removed-*")
	if len(matches) != 1 {
		t.Fatalf("expected one retired store beside the original, got %v", matches)
	}
}

func TestRemoveAccountRetiresProfileRecoverablyAndFreesTheSlot(t *testing.T) {
	withAccountRegistryFile(t)
	storeRoot := withAccountWebKitStoreRoot(t)
	registry, _ := loadAccountRegistry()
	defaultID := registry.Accounts[0].ID
	work, err := createAccount("Work")
	if err != nil {
		t.Fatal(err)
	}
	// Give Work some on-disk session material in every place it may live.
	if _, err := switchActiveAccount(work.ID, ""); err != nil {
		t.Fatal(err)
	}
	profileDir, err := activeAccountDataPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "Cookies"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(storeRoot, work.ID), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := switchActiveAccount(defaultID, ""); err != nil {
		t.Fatal(err)
	}

	removed, err := removeAccount(work.ID)
	if err != nil {
		t.Fatalf("an inactive second account must be removable: %v", err)
	}
	if removed.ID != work.ID || removed.Label != "Work" {
		t.Fatalf("unexpected removed account %#v", removed)
	}
	after, _ := loadAccountRegistry()
	if len(after.Accounts) != 1 || after.Accounts[0].ID != defaultID || after.ActiveAccountID != defaultID {
		t.Fatalf("registry after remove must hold only the untouched default account: %#v", after)
	}
	if _, err := os.Stat(profileDir); !os.IsNotExist(err) {
		t.Fatalf("the removed account's profile must no longer be at its live path (err=%v)", err)
	}
	moved, _ := filepath.Glob(profileDir + ".removed-*")
	if len(moved) != 1 {
		t.Fatalf("profile must be set aside recoverably, not deleted; found %v", moved)
	}
	if _, err := os.Stat(filepath.Join(moved[0], "Cookies")); err != nil {
		t.Fatalf("set-aside profile lost its contents: %v", err)
	}
	if _, err := os.Stat(legacyProfileDir()); err == nil {
		// The legacy dir may legitimately exist on a dev machine; it just must
		// never have been renamed by this test.
		if m, _ := filepath.Glob(legacyProfileDir() + ".removed-*"); len(m) > 0 {
			t.Fatalf("the legacy profile was touched: %v", m)
		}
	}

	// The slot is free again and a re-added account starts from a new id,
	// i.e. a clean profile rather than the retired one.
	again, err := createAccount("Work")
	if err != nil {
		t.Fatalf("a removed account's slot must be reusable: %v", err)
	}
	if again.ID == work.ID {
		t.Fatal("a re-added account must get a fresh id and therefore a fresh profile")
	}
}

func TestRetireAccountProfileDataRefusesPathTraversal(t *testing.T) {
	for _, id := range []string{"", ".", "..", "../x", `..\x`, "a/b", "C:x"} {
		if isSafeAccountPathElement(id) {
			t.Errorf("id %q must not be accepted as a path element", id)
		}
	}
	if !isSafeAccountPathElement(newAccountID()) {
		t.Error("a canonical account id must be accepted")
	}
}

func TestAccountProfilesLiveBesideTheRegistry(t *testing.T) {
	withAccountRegistryFile(t)
	if got, want := accountProfilesRoot(), filepath.Join(filepath.Dir(accountRegistryFilePath()), "Profiles"); got != want {
		t.Fatalf("profiles root %q must sit beside accounts.json (%q)", got, want)
	}
}

func TestNotificationTitleNamesTheAccountOnlyWithTwoAccounts(t *testing.T) {
	withAccountRegistryFile(t)
	if _, err := loadAccountRegistry(); err != nil {
		t.Fatal(err)
	}
	if got := labelNotificationTitle("Alice"); got != "Alice" {
		t.Fatalf("single-account notifications must be untouched, got %q", got)
	}
	work, err := createAccount("Work")
	if err != nil {
		t.Fatal(err)
	}
	if got := labelNotificationTitle("Alice"); got != "Default · Alice" {
		t.Fatalf("with two accounts the active label must prefix the title, got %q", got)
	}
	if _, err := switchActiveAccount(work.ID, ""); err != nil {
		t.Fatal(err)
	}
	if got := labelNotificationTitle(appUpdateNotificationTitle); got != appUpdateNotificationTitle {
		t.Fatalf("the app's own update notice must not be attributed to an account, got %q", got)
	}
	if !strings.Contains(readProjectFile(t, "main.go"), "var notifTitle = '"+appUpdateNotificationTitle+"'") {
		t.Fatal("the page's update notification title drifted from appUpdateNotificationTitle")
	}
	if got := labelNotificationTitle(""); got != "Work" {
		t.Fatalf("an empty title must fall back to the label, got %q", got)
	}
}
