package main

import (
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
