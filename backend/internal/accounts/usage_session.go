package accounts

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// UsageSession survives APK/backend restarts. It contains no GPS or token.
// Receipt identifiers remain encrypted and are never sent to the web client.
type UsageSession struct {
	ID              string    `json:"id"`
	Device          string    `json:"device"`
	SKU             string    `json:"sku"`
	Category        string    `json:"category"`
	Service         string    `json:"service"`
	StartedAt       time.Time `json:"started_at"`
	FinishedAt      time.Time `json:"finished_at,omitempty"`
	OrderNo         string    `json:"order_no,omitempty"`
	MessageID       string    `json:"message_id,omitempty"`
	SyncID          string    `json:"sync_id,omitempty"`
	State           string    `json:"state"`
	Confirmed       bool      `json:"confirmed"`
	ObservedRunning bool      `json:"observed_running"`
	StopAttempted   bool      `json:"stop_attempted"`
	LastCheckedAt   time.Time `json:"last_checked_at,omitempty"`
}

func (s *Store) readUsageSessions() (map[string]UsageSession, error) {
	raw, err := os.ReadFile(filepath.Join(s.dir, "usage-sessions.enc"))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]UsageSession{}, nil
	}
	if err != nil {
		return nil, err
	}
	n := s.aead.NonceSize()
	if len(raw) < n {
		return nil, errors.New("使用会话损坏")
	}
	plain, err := s.aead.Open(nil, raw[:n], raw[n:], []byte("panguai/usage-sessions/v1"))
	if err != nil {
		return nil, err
	}
	entries := map[string]UsageSession{}
	if err = json.Unmarshal(plain, &entries); err != nil || entries == nil {
		return nil, errors.New("使用会话格式错误")
	}
	return entries, nil
}
func (s *Store) writeUsageEncrypted(name, aad string, value any) error {
	plain, err := json.Marshal(value)
	if err != nil {
		return err
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	raw := s.aead.Seal(nonce, nonce, plain, []byte(aad))
	f, err := os.CreateTemp(s.dir, ".usage-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(s.dir, name))
}
func (s *Store) GetUsageSession(account, device string) (UsageSession, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.readUsageSessions()
	if err != nil {
		return UsageSession{}, false, err
	}
	entry, ok := entries[usageKey(account, device)]
	return entry, ok, nil
}
func (s *Store) CreateUsageSession(account, device string, entry UsageSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.readUsageSessions()
	if err != nil {
		return err
	}
	key := usageKey(account, device)
	if old, ok := entries[key]; ok && old.State != "finished" {
		return errors.New("该设备已有使用会话")
	}
	if entry.ID == "" || entry.Device != device || entry.SKU == "" {
		return errors.New("使用会话不完整")
	}
	// Keep recent settled receipts for recovery without unbounded growth.
	for k, v := range entries {
		if v.State == "finished" && time.Since(v.FinishedAt) > 7*24*time.Hour {
			delete(entries, k)
		}
	}
	entries[key] = entry
	return s.writeUsageEncrypted("usage-sessions.enc", "panguai/usage-sessions/v1", entries)
}
func (s *Store) UpdateUsageSession(account, device, id string, update func(*UsageSession) error) (UsageSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.readUsageSessions()
	if err != nil {
		return UsageSession{}, err
	}
	key := usageKey(account, device)
	entry, ok := entries[key]
	if !ok || entry.ID != id {
		return UsageSession{}, errors.New("使用会话已经变化")
	}
	if err = update(&entry); err != nil {
		return entry, err
	}
	entries[key] = entry
	return entry, s.writeUsageEncrypted("usage-sessions.enc", "panguai/usage-sessions/v1", entries)
}

// Only a verified terminal session may clear its persistent duplicate guard.
func (s *Store) ReleaseUsageAttempt(account, device string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.usageAttempts()
	if err != nil {
		return err
	}
	delete(entries, usageKey(account, device))
	return s.writeUsageEncrypted("usage-attempts.enc", "panguai/usage-attempts/v1", entries)
}
