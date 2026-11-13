package accounts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUsageAttemptSurvivesRestartAndContainsNoPlainIdentifiers(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := s.ReserveUsageAttempt("fixture-account", "fixture-device"); err != nil || !ok {
		t.Fatal("reservation failed", err)
	}
	other, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := other.ReserveUsageAttempt("fixture-account", "fixture-device"); err != nil || ok {
		t.Fatal("restart permitted a duplicate attempt")
	}
	if pending, err := other.UsageAttemptPending("fixture-account", "fixture-device"); err != nil || !pending {
		t.Fatal("attempt missing after reopen")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "usage-attempts.enc"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "fixture-") {
		t.Fatal("plain account or device saved")
	}
	if err := os.WriteFile(filepath.Join(dir, "usage-attempts.enc"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := other.ReserveUsageAttempt("fixture-account", "another-device"); err == nil {
		t.Fatal("damaged journal allowed a command")
	}
}
