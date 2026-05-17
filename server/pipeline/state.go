package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// StateAccessor provides read/write access to a job's key-value state store.
// Keys are typically step names; values are JSON-encoded bytes.
type StateAccessor interface {
	// Get returns the raw bytes for key, or os.ErrNotExist when absent.
	Get(key string) ([]byte, error)
	// Set stores raw bytes for key, overwriting any previous value.
	Set(key string, data []byte) error
	// Has reports whether key has been set.
	Has(key string) bool
}

// StateManager creates and releases per-job StateAccessors.
type StateManager interface {
	Open(jobID uint) (StateAccessor, error)
	Close(jobID uint) error
}

// GetState reads and deserializes a value of type T from state at key.
func GetState[T any](state StateAccessor, key string) (T, error) {
	var result T
	data, err := state.Get(key)
	if err != nil {
		return result, fmt.Errorf("state.Get(%q): %w", key, err)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("state.Unmarshal(%q): %w", key, err)
	}
	return result, nil
}

// SetState serializes a value of type T and writes it to state at key.
func SetState[T any](state StateAccessor, key string, value T) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("state.Marshal(%q): %w", key, err)
	}
	return state.Set(key, data)
}

// FileStateManager is the default StateManager, persisting state as JSON files on disk.
// Directory layout: <dir>/<jobID>/<key>.json
type FileStateManager struct{ dir string }

// NewFileStateManager creates a FileStateManager that stores state under dir.
func NewFileStateManager(dir string) *FileStateManager {
	return &FileStateManager{dir: dir}
}

// Open creates the per-job directory and returns a StateAccessor for it.
func (m *FileStateManager) Open(jobID uint) (StateAccessor, error) {
	d := filepath.Join(m.dir, fmt.Sprintf("%d", jobID))
	if err := os.MkdirAll(d, 0755); err != nil {
		return nil, fmt.Errorf("pipeline: mkdir %q: %w", d, err)
	}
	return &fileStateAccessor{dir: d}, nil
}

// Close is a no-op; file state persists on disk for crash recovery.
func (m *FileStateManager) Close(_ uint) error { return nil }

// Purge removes all state files for a job.
func (m *FileStateManager) Purge(jobID uint) error {
	return os.RemoveAll(filepath.Join(m.dir, fmt.Sprintf("%d", jobID)))
}

type fileStateAccessor struct{ dir string }

func (a *fileStateAccessor) Get(key string) ([]byte, error) {
	return os.ReadFile(a.path(key))
}

func (a *fileStateAccessor) Set(key string, data []byte) error {
	return os.WriteFile(a.path(key), data, 0644)
}

func (a *fileStateAccessor) Has(key string) bool {
	_, err := os.Stat(a.path(key))
	return err == nil
}

func (a *fileStateAccessor) path(key string) string {
	return filepath.Join(a.dir, key+".json")
}
