package xkeen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// PoolState remembers what the panel changed when it built a pool.
//
// OriginalTag is the reason this is persisted: leaving pool mode has to restore
// the tag the owner's routing rules were written against, and once the pool is
// in place that tag exists nowhere in the config anymore.
type PoolState struct {
	Enabled     bool   `json:"enabled"`
	BalancerTag string `json:"balancer_tag,omitempty"`
	Selector    string `json:"selector,omitempty"`
	OriginalTag string `json:"original_tag,omitempty"`
	RoutingFile string `json:"routing_file,omitempty"`
	APIFile     string `json:"api_file,omitempty"`   // api config the panel created, removed when leaving pool mode
	PinnedTag   string `json:"pinned_tag,omitempty"` // node pinned by hand via the balancer API
}

// PoolStore persists PoolState next to the panel's other data.
type PoolStore struct {
	dataDir string
	mu      sync.RWMutex
	state   PoolState
}

func NewPoolStore(dataDir string) *PoolStore {
	return &PoolStore{dataDir: dataDir}
}

func (s *PoolStore) filePath() string {
	return filepath.Join(s.dataDir, "pool.json")
}

// Load reads the stored state. A missing file is not an error — it just means
// the panel has never built a pool here.
func (s *PoolStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return json.Unmarshal(data, &s.state)
}

func (s *PoolStore) Get() PoolState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

func (s *PoolStore) Set(state PoolState) error {
	s.mu.Lock()
	s.state = state
	data, err := json.MarshalIndent(state, "", "  ")
	s.mu.Unlock()

	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dataDir, 0700); err != nil {
		return err
	}

	return os.WriteFile(s.filePath(), data, 0600)
}

// SetPinned records the hand-picked node so it can be re-applied after a restart —
// a balancer override lives only in Xray's memory.
func (s *PoolStore) SetPinned(tag string) error {
	state := s.Get()
	state.PinnedTag = tag
	return s.Set(state)
}
