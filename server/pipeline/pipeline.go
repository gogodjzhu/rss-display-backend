package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// Pipeline is a named, ordered, immutable sequence of steps.
type Pipeline struct {
	name  string
	steps []Step
}

// New creates a Pipeline. Steps execute in registration order.
func New(name string, steps ...Step) *Pipeline {
	return &Pipeline{name: name, steps: steps}
}

// Name returns the pipeline name.
func (p *Pipeline) Name() string { return p.name }

// Runner drives Pipeline execution: handles timeouts, retries, persistence, and crash recovery.
type Runner struct {
	store  JobStore
	state  StateManager
	pipes  map[string]*Pipeline
	logger *log.Logger
}

// NewRunner creates a Runner backed by the given store and state manager.
func NewRunner(store JobStore, state StateManager, logger *log.Logger) *Runner {
	return &Runner{
		store:  store,
		state:  state,
		pipes:  make(map[string]*Pipeline),
		logger: logger,
	}
}

// Register adds a Pipeline. Returns an error if a pipeline with the same name is already registered.
func (r *Runner) Register(p *Pipeline) error {
	if _, ok := r.pipes[p.Name()]; ok {
		return fmt.Errorf("pipeline: %q already registered", p.Name())
	}
	r.pipes[p.Name()] = p
	return nil
}

// Submit creates and enqueues a new job, then executes it asynchronously.
// Returns the persisted JobRecord (with ID populated) immediately.
func (r *Runner) Submit(ctx context.Context, pipelineName string, input json.RawMessage) (*JobRecord, error) {
	if _, ok := r.pipes[pipelineName]; !ok {
		return nil, fmt.Errorf("pipeline: %q not registered", pipelineName)
	}
	now := time.Now()
	job := &JobRecord{
		PipelineName: pipelineName,
		Status:       JobPending,
		Input:        input,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := r.store.Create(job); err != nil {
		return nil, fmt.Errorf("pipeline: create job: %w", err)
	}
	go r.run(context.Background(), job.ID)
	return job, nil
}

// Recover resumes all Pending or Running jobs; call once on process restart before serving traffic.
func (r *Runner) Recover(ctx context.Context) error {
	jobs, err := r.store.LoadPending()
	if err != nil {
		return fmt.Errorf("pipeline: load pending jobs: %w", err)
	}
	for _, job := range jobs {
		r.logger.Printf("[pipeline] recovering job %d (%s step=%d)", job.ID, job.PipelineName, job.CurrentStep)
		go r.run(context.Background(), job.ID)
	}
	return nil
}

// run is the main execution loop; runs in a goroutine.
func (r *Runner) run(ctx context.Context, jobID uint) {
	job, err := r.store.Load(jobID)
	if err != nil {
		r.logger.Printf("[pipeline] load job %d: %v", jobID, err)
		return
	}

	pipe, ok := r.pipes[job.PipelineName]
	if !ok {
		r.fail(job, fmt.Sprintf("pipeline %q not registered", job.PipelineName))
		return
	}

	state, err := r.state.Open(jobID)
	if err != nil {
		r.fail(job, fmt.Sprintf("open state: %v", err))
		return
	}
	defer r.state.Close(jobID)

	// Write job input to state so all steps can access it.
	// On crash recovery (CurrentStep > 0), skip if already written.
	if !state.Has("job_input") {
		if err := state.Set("job_input", job.Input); err != nil {
			r.fail(job, fmt.Sprintf("init job_input: %v", err))
			return
		}
	}
	// Store the job ID in state for steps that need to write back to the job record.
	if !state.Has("job_id") {
		idBytes, _ := json.Marshal(jobID)
		if err := state.Set("job_id", idBytes); err != nil {
			r.fail(job, fmt.Sprintf("init job_id: %v", err))
			return
		}
	}

	for i := job.CurrentStep; i < len(pipe.steps); i++ {
		step := pipe.steps[i]
		cfg := step.Config()

		job.Status = JobRunning
		job.CurrentStep = i
		job.UpdatedAt = time.Now()
		_ = r.store.Save(job)

		stepCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		runErr := r.runWithRetry(stepCtx, step, state, cfg.RetryPolicy)
		cancel()

		if runErr != nil {
			r.fail(job, fmt.Sprintf("step %q: %v", step.Name(), runErr))
			return
		}
	}

	now := time.Now()
	job.Status = JobCompleted
	job.CompletedAt = &now
	job.UpdatedAt = now
	_ = r.store.Save(job)
	r.logger.Printf("[pipeline] job %d completed", jobID)
}

// runWithRetry executes a step with exponential-backoff retries.
// A MaxAttempts value less than 1 is treated as 1 (run at least once).
func (r *Runner) runWithRetry(ctx context.Context, step Step, state StateAccessor, p RetryPolicy) error {
	if p.MaxAttempts < 1 {
		p.MaxAttempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= p.MaxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := step.Run(ctx, state); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if attempt < p.MaxAttempts {
			delay := p.BaseDelay * (1 << uint(attempt-1))
			if delay > p.MaxDelay {
				delay = p.MaxDelay
			}
			r.logger.Printf("[pipeline] step %q attempt %d/%d failed: %v, retrying in %v",
				step.Name(), attempt, p.MaxAttempts, lastErr, delay)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return lastErr
}

func (r *Runner) fail(job *JobRecord, msg string) {
	job.Status = JobFailed
	job.Error = msg
	job.UpdatedAt = time.Now()
	_ = r.store.Save(job)
	r.logger.Printf("[pipeline] job %d failed: %s", job.ID, msg)
}
