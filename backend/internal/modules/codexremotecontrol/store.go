package codexremotecontrol

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

type MemoryStore struct {
	mu     sync.Mutex
	record *Record
}

func (s *MemoryStore) Load(context.Context) (*Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.record == nil {
		return nil, nil
	}
	copy := *s.record
	return &copy, nil
}
func (s *MemoryStore) Save(_ context.Context, record Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := record
	s.record = &copy
	return nil
}
func (s *MemoryStore) Clear(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.record = nil
	return nil
}

// FileStore persists public enrollment references under an installation-bound
// key. Native private key material and remote-control tokens are not stored here.
type FileStore struct {
	path string
	mu   sync.Mutex
}

func NewFileStore(directory, baseURL, accountUserID, installationID string) (*FileStore, error) {
	if !filepath.IsAbs(directory) || baseURL == "" || accountUserID == "" || installationID == "" {
		return nil, errors.New("device enrollment store scope is incomplete")
	}
	encoded, err := json.Marshal([]string{baseURL, accountUserID, installationID})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(encoded)
	return &FileStore{path: filepath.Join(directory, hex.EncodeToString(digest[:])+".json")}, nil
}

func (s *FileStore) Load(ctx context.Context) (*Record, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	info, err := os.Lstat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 16*1024 {
		return nil, errors.New("invalid device enrollment record")
	}
	body, err := os.ReadFile(s.path)
	if err != nil {
		return nil, err
	}
	var record Record
	if err := json.Unmarshal(body, &record); err != nil {
		return nil, errors.New("malformed device enrollment record")
	}
	return &record, nil
}

func (s *FileStore) Save(ctx context.Context, record Record) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	body, err := json.Marshal(record)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(s.path), ".enrollment-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), s.path)
}

func (s *FileStore) Clear(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
