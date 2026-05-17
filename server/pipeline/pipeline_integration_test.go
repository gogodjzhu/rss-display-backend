package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ── pipeline_integration_test.go ────────────────────────────────────────────
// These tests use only in-memory mocks; no database or Python required.

func TestPipelineNewAndName(t *testing.T) {
	p := New("mypipe")
	if p.Name() != "mypipe" {
		t.Errorf("Name() = %q, want %q", p.Name(), "mypipe")
	}
}

func TestMemJobStoreCreateAndLoad(t *testing.T) {
	store := newMemJobStore()
	job := &JobRecord{
		PipelineName: "pipe",
		Status:       JobPending,
		Input:        json.RawMessage(`{}`),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := store.Create(job); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if job.ID == 0 {
		t.Error("Create should set job.ID")
	}

	loaded, err := store.Load(job.ID)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.PipelineName != "pipe" {
		t.Errorf("PipelineName = %q, want %q", loaded.PipelineName, "pipe")
	}
	if loaded.Status != JobPending {
		t.Errorf("Status = %q, want %q", loaded.Status, JobPending)
	}
}

func TestMemJobStoreSaveUpdatesStatus(t *testing.T) {
	store := newMemJobStore()
	job := &JobRecord{PipelineName: "p", Status: JobPending, Input: json.RawMessage(`{}`), CreatedAt: time.Now(), UpdatedAt: time.Now()}
	_ = store.Create(job)

	job.Status = JobCompleted
	now := time.Now()
	job.CompletedAt = &now
	_ = store.Save(job)

	loaded, _ := store.Load(job.ID)
	if loaded.Status != JobCompleted {
		t.Errorf("saved status = %q, want %q", loaded.Status, JobCompleted)
	}
	if loaded.CompletedAt == nil {
		t.Error("saved CompletedAt should not be nil")
	}
}

func TestMemJobStoreLoadPendingFilters(t *testing.T) {
	store := newMemJobStore()

	pending := &JobRecord{PipelineName: "p", Status: JobPending, Input: json.RawMessage(`{}`), CreatedAt: time.Now(), UpdatedAt: time.Now()}
	running := &JobRecord{PipelineName: "p", Status: JobRunning, Input: json.RawMessage(`{}`), CreatedAt: time.Now(), UpdatedAt: time.Now()}
	done := &JobRecord{PipelineName: "p", Status: JobCompleted, Input: json.RawMessage(`{}`), CreatedAt: time.Now(), UpdatedAt: time.Now()}
	failed := &JobRecord{PipelineName: "p", Status: JobFailed, Input: json.RawMessage(`{}`), CreatedAt: time.Now(), UpdatedAt: time.Now()}

	for _, j := range []*JobRecord{pending, running, done, failed} {
		_ = store.Create(j)
	}

	results, err := store.LoadPending()
	if err != nil {
		t.Fatalf("LoadPending failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("LoadPending returned %d records, want 2 (pending+running)", len(results))
	}
}

func TestMemJobStoreLoadMissingReturnsError(t *testing.T) {
	store := newMemJobStore()
	_, err := store.Load(999)
	if err == nil {
		t.Error("expected error loading non-existent job")
	}
}

func TestFileStateManagerMultipleJobs(t *testing.T) {
	fsm := newTempStateManager(t)

	acc1, _ := fsm.Open(1)
	acc2, _ := fsm.Open(2)

	_ = acc1.Set("k", []byte(`"job1"`))
	_ = acc2.Set("k", []byte(`"job2"`))

	v1, _ := acc1.Get("k")
	v2, _ := acc2.Get("k")

	if string(v1) != `"job1"` {
		t.Errorf("job1 state = %q, want %q", v1, `"job1"`)
	}
	if string(v2) != `"job2"` {
		t.Errorf("job2 state = %q, want %q", v2, `"job2"`)
	}
}

func TestFileStateManagerStoredOnDisk(t *testing.T) {
	dir := t.TempDir()
	fsm := NewFileStateManager(dir)

	acc, _ := fsm.Open(42)
	_ = acc.Set("mykey", []byte(`123`))

	// Verify the file exists at the expected path.
	expectedPath := filepath.Join(dir, "42", "mykey.json")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("expected state file at %s", expectedPath)
	}
}

func TestSetAndGetStateSlice(t *testing.T) {
	fsm := newTempStateManager(t)
	acc, _ := fsm.Open(100)

	ids := []uint{1, 2, 3, 4}
	if err := SetState(acc, "ids", ids); err != nil {
		t.Fatalf("SetState failed: %v", err)
	}

	got, err := GetState[[]uint](acc, "ids")
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if len(got) != len(ids) {
		t.Errorf("got %d ids, want %d", len(got), len(ids))
	}
	for i, id := range got {
		if id != ids[i] {
			t.Errorf("ids[%d] = %d, want %d", i, id, ids[i])
		}
	}
}
