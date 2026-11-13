package accounts

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestVaultPersistenceAndTamperDetection(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	r := Record{Profile: Profile{ID: "123", Name: "测试用户", Phone: "138****0000"}, Token: "test-sensitive-platform-token"}
	if err := s.Save(r); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "accounts.enc"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(r.Token)) || bytes.Contains(raw, []byte(r.Profile.Name)) {
		t.Fatal("account material leaked in plaintext")
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Get("123")
	if err != nil || got.Token != r.Token || got.Profile.UpdatedAt.IsZero() {
		t.Fatal("saved credential did not survive reopen", err)
	}
	raw[len(raw)-1] ^= 1
	if err := os.WriteFile(filepath.Join(dir, "accounts.enc"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("tampered ciphertext was accepted")
	}
}

func TestMissingKeyCannotOverwriteExistingVault(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save(Record{Profile: Profile{ID: "a"}, Token: "token"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "accounts.key")); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("missing key silently replaced")
	}
	if _, err := os.Stat(filepath.Join(dir, "accounts.key")); !os.IsNotExist(err) {
		t.Fatal("a replacement key was created")
	}
}

func TestConcurrentSavesAndLocalRemoval(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, id := range []string{"a", "b", "c", "d"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if err := s.Save(Record{Profile: Profile{ID: id}, Token: "test-" + id}); err != nil {
				t.Error(err)
			}
		}(id)
	}
	wg.Wait()
	if err := s.Remove("b"); err != nil {
		t.Fatal(err)
	}
	list, err := s.List()
	if err != nil || len(list) != 3 {
		t.Fatal("account update lost records", len(list), err)
	}
	if _, err := s.Get("b"); err == nil {
		t.Fatal("removed credential is still available")
	}
}
