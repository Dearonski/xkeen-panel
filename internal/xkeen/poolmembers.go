package xkeen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// MemberStore keeps the servers the owner put into the pool by hand, keyed by endpoint since server IDs shift on every subscription refresh.
type MemberStore struct {
	path string
	mu   sync.RWMutex
	keys map[string]bool
}

func NewMemberStore(dataDir string) *MemberStore {
	return &MemberStore{path: filepath.Join(dataDir, "pool_members.json"), keys: map[string]bool{}}
}

func (s *MemberStore) Load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var keys []string
	if err := json.Unmarshal(data, &keys); err != nil {
		return err
	}

	s.mu.Lock()
	s.keys = make(map[string]bool, len(keys))
	for _, key := range keys {
		s.keys[key] = true
	}
	s.mu.Unlock()
	return nil
}

func (s *MemberStore) Keys() map[string]bool {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make(map[string]bool, len(s.keys))
	for key := range s.keys {
		keys[key] = true
	}
	return keys
}

func (s *MemberStore) Has(key string) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.keys[key]
}

func (s *MemberStore) Set(key string, keep bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if keep {
		s.keys[key] = true
	} else {
		delete(s.keys, key)
	}
	list := make([]string, 0, len(s.keys))
	for k := range s.keys {
		list = append(list, k)
	}

	sort.Strings(list)
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
}
