package accounts

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// The encrypted attempt journal prevents a backend restart from immediately
// repeating an ambiguous physical command. No location, token or fee is saved.
func usageKey(account, device string) string {
	hash := sha256.Sum256([]byte(account + "\x00" + device))
	return hex.EncodeToString(hash[:])
}
func (s *Store) usageAttempts() (map[string]time.Time, error) {
	raw, err := os.ReadFile(filepath.Join(s.dir, "usage-attempts.enc"))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]time.Time{}, nil
	}
	if err != nil {
		return nil, err
	}
	n := s.aead.NonceSize()
	if len(raw) < n {
		return nil, errors.New("启动记录损坏")
	}
	plain, err := s.aead.Open(nil, raw[:n], raw[n:], []byte("panguai/usage-attempts/v1"))
	if err != nil {
		return nil, err
	}
	entries := map[string]time.Time{}
	if err := json.Unmarshal(plain, &entries); err != nil || entries == nil {
		return nil, errors.New("启动记录格式错误")
	}
	return entries, nil
}
func (s *Store) UsageAttemptPending(account, device string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.usageAttempts()
	if err != nil {
		return false, err
	}
	return time.Now().Before(entries[usageKey(account, device)]), nil
}
func (s *Store) ReserveUsageAttempt(account, device string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.usageAttempts()
	if err != nil {
		return false, err
	}
	now := time.Now()
	key := usageKey(account, device)
	if now.Before(entries[key]) {
		return false, nil
	}
	for k, expiry := range entries {
		if now.After(expiry) {
			delete(entries, k)
		}
	}
	entries[key] = now.Add(32 * time.Minute)
	plain, err := json.Marshal(entries)
	if err != nil {
		return false, err
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return false, err
	}
	raw := s.aead.Seal(nonce, nonce, plain, []byte("panguai/usage-attempts/v1"))
	f, err := os.CreateTemp(s.dir, ".usage-*")
	if err != nil {
		return false, err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return false, err
	}
	if _, err := f.Write(raw); err != nil {
		f.Close()
		return false, err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return false, err
	}
	if err := f.Close(); err != nil {
		return false, err
	}
	if err := os.Rename(f.Name(), filepath.Join(s.dir, "usage-attempts.enc")); err != nil {
		return false, err
	}
	return true, nil
}
