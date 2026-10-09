package cli

import (
	"os"
	"path/filepath"
	"testing"
)

const testCredential = "buyer-credential-0123456789"

func TestTheCredentialIsKeptInAFileOnlyTheOwnerCanRead(t *testing.T) {
	env := &Env{ConfigDir: t.TempDir()}
	store := env.credentials()
	if got, err := store.Load("https://api.example"); err != nil {
		t.Fatalf("an empty store holds nothing, got %q (%v)", got, err)
	} else if got != "" {
		t.Fatalf("an empty store holds nothing, got %q (%v)", got, err)
	}
	if err := store.Save("https://api.example/", testCredential); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(env.ConfigDir, credentialFile)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("the credential file mode is %v, want 0600", info.Mode().Perm())
	}
	if got, _ := store.Load("https://api.example"); got != testCredential {
		t.Fatalf("a trailing slash names the same origin, got %q", got)
	}
	if got, _ := store.Load("https://other.example"); got != "" {
		t.Fatal("a credential belongs to one origin")
	}
	if err := store.Delete("https://api.example"); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Load("https://api.example"); got != "" {
		t.Fatal("a deleted credential is gone")
	}
	if err := store.Delete("https://api.example"); err == nil {
		t.Fatal("deleting a credential that is not stored is an error")
	}
}

func TestACredentialFileThatAnotherUserCanReadIsRefused(t *testing.T) {
	env := &Env{ConfigDir: t.TempDir()}
	if err := env.credentials().Save("https://api.example", testCredential); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(env.ConfigDir, credentialFile), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := env.credentials().Load("https://api.example"); err == nil {
		t.Fatal("a credential in a file that another user can read is no longer private")
	}
}

func TestAMalformedCredentialIsNeverStored(t *testing.T) {
	env := &Env{ConfigDir: t.TempDir()}
	for _, credential := range []string{"", "short", "has a space in the credential value", testCredential + "\n"} {
		if err := env.credentials().Save("https://api.example", credential); err == nil {
			t.Fatalf("%q was stored", credential)
		}
	}
}

func TestThePurchaseRecordSurvivesARestartAndIsFoundByItsOrder(t *testing.T) {
	env := &Env{ConfigDir: t.TempDir()}
	if _, found, err := loadRecord(env, "quo_1"); found {
		t.Fatalf("no record exists yet: %v", err)
	} else if err != nil {
		t.Fatalf("no record exists yet: %v", err)
	}
	record := purchaseRecord{QuoteID: "quo_1", Key: "key-1", PaymentID: "pay_0123456789abcdef", Signature: "SIGNED", OrderID: "ord_1"}
	if err := saveRecord(env, record); err != nil {
		t.Fatal(err)
	}
	got, found, err := loadRecord(env, "quo_1")
	if err != nil {
		t.Fatalf("the record is read back unchanged, got %+v (%v)", got, err)
	}
	if !found {
		t.Fatalf("the record is read back unchanged, got %+v (%v)", got, err)
	}
	if got != record {
		t.Fatalf("the record is read back unchanged, got %+v (%v)", got, err)
	}
	if byOrder, ok := findRecordByOrder(env, "ord_1"); !ok {
		t.Fatal("the record is found by its order")
	} else if byOrder.QuoteID != "quo_1" {
		t.Fatal("the record is found by its order")
	}
	info, _ := os.Stat(filepath.Join(env.ConfigDir, "purchases", "quo_1.json"))
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("the record mode is %v, want 0600", info.Mode().Perm())
	}
	for _, unsafe := range []string{"../escape", "a/b", "", "quo 1"} {
		if _, _, err := loadRecord(env, unsafe); err == nil {
			t.Fatalf("the quote ID %q must be refused as a file name", unsafe)
		}
	}
}
