package pipeline

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"testing"
	"time"
)

// ── memJobStore ───────────────────────────────────────────────────────────────

type memJobStore struct {
	mu     sync.Mutex
	jobs   map[uint]*JobRecord
	nextID uint
}

func newMemJobStore() *memJobStore {
	return &memJobStore{jobs: make(map[uint]*JobRecord), nextID: 1}
}

func (s *memJobStore) Create(job *JobRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job.ID = s.nextID
	s.nextID++
	cp := *job
	s.jobs[job.ID] = &cp
	return nil
}

func (s *memJobStore) Save(job *JobRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *job
	s.jobs[job.ID] = &cp
	return nil
}

func (s *memJobStore) Load(id uint) (*JobRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return nil, fmt.Errorf("job %d not found", id)
	}
	cp := *job
	return &cp, nil
}

func (s *memJobStore) LoadPending() ([]*JobRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*JobRecord
	for _, job := range s.jobs {
		if job.Status == JobPending || job.Status == JobRunning {
			cp := *job
			out = append(out, &cp)
		}
	}
	return out, nil
}

// ── memStateManager ───────────────────────────────────────────────────────────

type memStateManager struct {
	mu     sync.Mutex
	states map[uint]map[string][]byte
}

func newMemStateManager() *memStateManager {
	return &memStateManager{states: make(map[uint]map[string][]byte)}
}

func (m *memStateManager) Open(jobID uint) (StateAccessor, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.states[jobID]; !exists {
		m.states[jobID] = make(map[string][]byte)
	}
	return &memStateAccessor{data: m.states[jobID], mu: &m.mu}, nil
}

func (m *memStateManager) Close(_ uint) error { return nil }

type memStateAccessor struct {
	data map[string][]byte
	mu   *sync.Mutex
}

func (a *memStateAccessor) Get(key string) ([]byte, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	d, ok := a.data[key]
	if !ok {
		return nil, os.ErrNotExist
	}
	return d, nil
}

func (a *memStateAccessor) Set(key string, data []byte) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.data[key] = data
	return nil
}

func (a *memStateAccessor) Has(key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.data[key]
	return ok
}

// ── noopStep ─────────────────────────────────────────────────────────────────

type noopStep struct {
	stepName string
	ran      *bool
}

func newNoopStep(name string, ran *bool) *noopStep {
	return &noopStep{stepName: name, ran: ran}
}

func (s *noopStep) Name() string { return s.stepName }
func (s *noopStep) Config() StepConfig {
	return StepConfig{Timeout: 5 * time.Second, RetryPolicy: RetryPolicy{MaxAttempts: 1}}
}
func (s *noopStep) Run(_ context.Context, _ StateAccessor) error {
	if s.ran != nil {
		*s.ran = true
	}
	return nil
}

// ── errStep ───────────────────────────────────────────────────────────────────

type errStep struct {
	stepName string
}

func (s *errStep) Name() string { return s.stepName }
func (s *errStep) Config() StepConfig {
	return StepConfig{Timeout: 5 * time.Second, RetryPolicy: RetryPolicy{MaxAttempts: 1}}
}
func (s *errStep) Run(_ context.Context, _ StateAccessor) error {
	return fmt.Errorf("errStep %q always fails", s.stepName)
}

// ── captureStep ───────────────────────────────────────────────────────────────

// captureStep calls a user-supplied function in Run; useful for white-box assertions.
type captureStep struct {
	stepName string
	fn       func(StateAccessor) error
	cfg      StepConfig
}

func (s *captureStep) Name() string { return s.stepName }
func (s *captureStep) Config() StepConfig {
	if s.cfg.Timeout == 0 {
		return StepConfig{Timeout: 5 * time.Second, RetryPolicy: RetryPolicy{MaxAttempts: 1}}
	}
	return s.cfg
}
func (s *captureStep) Run(_ context.Context, state StateAccessor) error { return s.fn(state) }

// ── newTempStateManager ───────────────────────────────────────────────────────

func newTempStateManager(t *testing.T) *FileStateManager {
	t.Helper()
	return NewFileStateManager(t.TempDir())
}

// ── testLogger ────────────────────────────────────────────────────────────────

func testLogger() *log.Logger { return log.Default() }
