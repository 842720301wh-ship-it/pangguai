package accounts

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Profile is the only account information returned to the browser.
type Profile struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Phone     string    `json:"phone,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Record struct {
	Profile    Profile       `json:"profile"`
	Token      string        `json:"token"`
	PlatformID string        `json:"platform_id,omitempty"`
	Devices    []SavedDevice `json:"devices,omitempty"`
}

type SavedDevice struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Category     string `json:"category"`
	Organization string `json:"organization,omitempty"`
}

type Store struct {
	mu   sync.Mutex
	dir  string
	aead cipher.AEAD
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	keyPath := filepath.Join(dir, "accounts.key")
	key, err := os.ReadFile(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		// Never replace a missing key when an encrypted account file already exists.
		if _, statErr := os.Stat(filepath.Join(dir, "accounts.enc")); statErr == nil {
			return nil, errors.New("账号密钥缺失，请恢复原密钥")
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return nil, statErr
		}
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return nil, err
		}
		f, openErr := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if openErr != nil {
			return nil, openErr
		}
		_, err = f.Write(key)
		closeErr := f.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
	} else if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, errors.New("账号密钥无效")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	s := &Store{dir: dir, aead: aead}
	if _, err := s.read(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) read() ([]Record, error) {
	raw, err := os.ReadFile(filepath.Join(s.dir, "accounts.enc"))
	if errors.Is(err, os.ErrNotExist) {
		return []Record{}, nil
	}
	if err != nil {
		return nil, err
	}
	n := s.aead.NonceSize()
	if len(raw) < n {
		return nil, errors.New("账号文件损坏")
	}
	plain, err := s.aead.Open(nil, raw[:n], raw[n:], []byte("panguai/accounts/v1"))
	if err != nil {
		return nil, errors.New("账号解密失败，请核对密钥和账号文件")
	}
	var records []Record
	if err := json.Unmarshal(plain, &records); err != nil {
		return nil, errors.New("账号文件格式错误")
	}
	for _, r := range records {
		if r.Profile.ID == "" || r.Token == "" {
			return nil, errors.New("账号记录不完整")
		}
	}
	return records, nil
}

func (s *Store) write(records []Record) error {
	plain, err := json.Marshal(records)
	if err != nil {
		return err
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	raw := s.aead.Seal(nonce, nonce, plain, []byte("panguai/accounts/v1"))
	f, err := os.CreateTemp(s.dir, ".accounts-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(s.dir, "accounts.enc"))
}

func (s *Store) List() ([]Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := s.read()
	if err != nil {
		return nil, err
	}
	profiles := make([]Profile, 0, len(records))
	for _, r := range records {
		profiles = append(profiles, r.Profile)
	}
	return profiles, nil
}

func (s *Store) Save(r Record) error {
	if r.Profile.ID == "" || r.Token == "" {
		return errors.New("拒绝保存未经确认的账号")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := s.read()
	if err != nil {
		return err
	}
	r.Profile.UpdatedAt = time.Now().UTC()
	for i := range records {
		if records[i].Profile.ID == r.Profile.ID {
			if r.Devices == nil {
				r.Devices = records[i].Devices
			}
			records[i] = r
			return s.write(records)
		}
	}
	return s.write(append(records, r))
}

func (s *Store) Get(id string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := s.read()
	if err != nil {
		return Record{}, err
	}
	for _, r := range records {
		if r.Profile.ID == id {
			return r, nil
		}
	}
	return Record{}, fmt.Errorf("账号不存在")
}

func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := s.read()
	if err != nil {
		return err
	}
	for i, r := range records {
		if r.Profile.ID == id {
			return s.write(append(records[:i], records[i+1:]...))
		}
	}
	return errors.New("账号不存在")
}
