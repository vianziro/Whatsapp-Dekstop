package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
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
