package pipeline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ── PythonRunner file I/O tests ─────────────────────────────────────────────

func TestPythonRunnerWriteAndReadJSON(t *testing.T) {
	runner := &PythonRunner{PythonPath: "python3", ScriptPath: "py/pipeline.py", DataDir: t.TempDir()}

	type payload struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	path := filepath.Join(t.TempDir(), "test.json")
	want := payload{Name: "test", Value: 42}

	if err := runner.WriteJSON(path, want); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	var got payload
	if err := runner.ReadJSON(path, &got); err != nil {
		t.Fatalf("ReadJSON failed: %v", err)
	}

	if got.Name != want.Name || got.Value != want.Value {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestPythonRunnerInputOutputPaths(t *testing.T) {
	dir := t.TempDir()
	runner := &PythonRunner{PythonPath: "python3", ScriptPath: "py/pipeline.py", DataDir: dir}

	inPath := runner.InputPath(7, "filter_l1")
	outPath := runner.OutputPath(7, "filter_l1")

	if inPath == outPath {
		t.Error("InputPath and OutputPath must be different")
	}
	if filepath.Dir(inPath) != dir {
		t.Errorf("InputPath dir = %q, want %q", filepath.Dir(inPath), dir)
	}
}

// ── StateAccessor (FileStateManager) tests ─────────────────────────────────

func TestFileStateManagerOpenAndClose(t *testing.T) {
	fsm := newTempStateManager(t)

	acc, err := fsm.Open(1)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if acc == nil {
		t.Fatal("Open returned nil accessor")
	}
	if err := fsm.Close(1); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestFileStateAccessorSetHasGet(t *testing.T) {
	fsm := newTempStateManager(t)
	acc, _ := fsm.Open(2)

	if acc.Has("foo") {
		t.Error("Has('foo') should be false before Set")
	}

	if err := acc.Set("foo", []byte(`"bar"`)); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if !acc.Has("foo") {
		t.Error("Has('foo') should be true after Set")
	}

	data, err := acc.Get("foo")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(data) != `"bar"` {
		t.Errorf("Get returned %q, want %q", data, `"bar"`)
	}
}

func TestFileStateAccessorGetMissingReturnsNotExist(t *testing.T) {
	fsm := newTempStateManager(t)
	acc, _ := fsm.Open(3)

	_, err := acc.Get("missing")
	if !os.IsNotExist(err) {
		t.Errorf("expected os.ErrNotExist, got %v", err)
	}
}

func TestFileStateManagerPurge(t *testing.T) {
	dir := t.TempDir()
	fsm := NewFileStateManager(dir)
	acc, _ := fsm.Open(5)
	_ = acc.Set("key", []byte("value"))

	stateDir := filepath.Join(dir, "5")
	if _, err := os.Stat(stateDir); os.IsNotExist(err) {
		t.Fatal("state dir should exist after Open+Set")
	}

	if err := fsm.Purge(5); err != nil {
		t.Fatalf("Purge failed: %v", err)
	}
	if _, err := os.Stat(stateDir); !os.IsNotExist(err) {
		t.Error("state dir should be removed after Purge")
	}
}

// ── GetState / SetState generics ────────────────────────────────────────────

func TestGetSetStateRoundTrip(t *testing.T) {
	fsm := newTempStateManager(t)
	acc, _ := fsm.Open(10)

	type point struct {
		X int `json:"x"`
		Y int `json:"y"`
	}
	want := point{X: 3, Y: 7}

	if err := SetState(acc, "point", want); err != nil {
		t.Fatalf("SetState failed: %v", err)
	}

	got, err := GetState[point](acc, "point")
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestGetStateMissingKeyReturnsError(t *testing.T) {
	fsm := newTempStateManager(t)
	acc, _ := fsm.Open(11)

	_, err := GetState[string](acc, "nonexistent")
	if err == nil {
		t.Error("expected error for missing key")
	}
}

// ── Runner tests ─────────────────────────────────────────────────────────────

func TestRunnerRegisterDuplicate(t *testing.T) {
	store := newMemJobStore()
	state := newMemStateManager()
	runner := NewRunner(store, state, testLogger())

	p := New("pipe1")
	if err := runner.Register(p); err != nil {
		t.Fatalf("first Register failed: %v", err)
	}
	if err := runner.Register(p); err == nil {
		t.Error("second Register with same name should fail")
	}
}

func TestRunnerSubmitUnknownPipelineReturnsError(t *testing.T) {
	store := newMemJobStore()
	state := newMemStateManager()
	runner := NewRunner(store, state, testLogger())

	_, err := runner.Submit(context.Background(), "no_such_pipeline", json.RawMessage(`{}`))
	if err == nil {
		t.Error("Submit for unregistered pipeline should return error")
	}
}

func TestRunnerSubmitCreatesJobAndRunsAllSteps(t *testing.T) {
	store := newMemJobStore()
	state := newMemStateManager()
	runner := NewRunner(store, state, testLogger())

	ran1, ran2 := false, false
	p := New("mypipe", newNoopStep("step1", &ran1), newNoopStep("step2", &ran2))
	_ = runner.Register(p)

	job, err := runner.Submit(context.Background(), "mypipe", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	if job.ID == 0 {
		t.Error("submitted job should have non-zero ID")
	}

	// Wait for async goroutine to finish.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rec, _ := store.Load(job.ID)
		if rec.Status == JobCompleted || rec.Status == JobFailed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	rec, _ := store.Load(job.ID)
	if rec.Status != JobCompleted {
		t.Errorf("job status = %q, want completed; error = %q", rec.Status, rec.Error)
	}
	if !ran1 || !ran2 {
		t.Errorf("not all steps ran: ran1=%v ran2=%v", ran1, ran2)
	}
}

func TestRunnerStepFailureSetsJobFailed(t *testing.T) {
	store := newMemJobStore()
	state := newMemStateManager()
	runner := NewRunner(store, state, testLogger())

	p := New("failpipe", &errStep{stepName: "bad_step"})
	_ = runner.Register(p)

	job, _ := runner.Submit(context.Background(), "failpipe", json.RawMessage(`{}`))

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rec, _ := store.Load(job.ID)
		if rec.Status == JobFailed || rec.Status == JobCompleted {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	rec, _ := store.Load(job.ID)
	if rec.Status != JobFailed {
		t.Errorf("job status = %q, want failed", rec.Status)
	}
	if rec.Error == "" {
		t.Error("failed job should have non-empty error message")
	}
}

func TestRunnerJobInputWrittenToState(t *testing.T) {
	store := newMemJobStore()
	stateManager := newMemStateManager()
	runner := NewRunner(store, stateManager, testLogger())

	type captured struct {
		gotInput bool
		gotID    bool
	}
	var result captured

	checkStep := &captureStep{fn: func(state StateAccessor) error {
		result.gotInput = state.Has("job_input")
		result.gotID = state.Has("job_id")
		return nil
	}}

	p := New("checkpipe", checkStep)
	_ = runner.Register(p)

	job, _ := runner.Submit(context.Background(), "checkpipe", json.RawMessage(`{"hello":"world"}`))

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rec, _ := store.Load(job.ID)
		if rec.Status == JobCompleted || rec.Status == JobFailed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !result.gotInput {
		t.Error("job_input should be in state")
	}
	if !result.gotID {
		t.Error("job_id should be in state")
	}
}

func TestRunnerRetryPolicyMaxAttemptsZeroRunsOnce(t *testing.T) {
	store := newMemJobStore()
	state := newMemStateManager()
	runner := NewRunner(store, state, testLogger())

	attempts := 0
	countStep := &captureStep{fn: func(_ StateAccessor) error {
		attempts++
		return nil
	}, cfg: StepConfig{
		Timeout:     5 * time.Second,
		RetryPolicy: RetryPolicy{MaxAttempts: 0}, // should be treated as 1
	}}

	p := New("retrypipe", countStep)
	_ = runner.Register(p)

	job, _ := runner.Submit(context.Background(), "retrypipe", json.RawMessage(`{}`))

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rec, _ := store.Load(job.ID)
		if rec.Status == JobCompleted || rec.Status == JobFailed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if attempts != 1 {
		t.Errorf("expected exactly 1 attempt with MaxAttempts=0, got %d", attempts)
	}
}

func TestRunnerRecoverResumesRunningJobs(t *testing.T) {
	store := newMemJobStore()
	state := newMemStateManager()

	// Pre-create a running job.
	job := &JobRecord{
		PipelineName: "recoverpipe",
		Status:       JobRunning,
		CurrentStep:  0,
		Input:        json.RawMessage(`{}`),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	_ = store.Create(job)

	ran := false
	runner := NewRunner(store, state, testLogger())
	p := New("recoverpipe", newNoopStep("s", &ran))
	_ = runner.Register(p)

	if err := runner.Recover(context.Background()); err != nil {
		t.Fatalf("Recover failed: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rec, _ := store.Load(job.ID)
		if rec.Status == JobCompleted || rec.Status == JobFailed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	rec, _ := store.Load(job.ID)
	if rec.Status != JobCompleted {
		t.Errorf("recovered job status = %q, want completed", rec.Status)
	}
}
