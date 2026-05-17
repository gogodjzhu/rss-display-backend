package pipeline

import (
	"encoding/json"
	"time"
)

// JobStore persists JobRecords across process restarts.
type JobStore interface {
	// Create writes a new job record and populates job.ID on success.
	Create(job *JobRecord) error
	// Save updates a job record's mutable fields (status, current_step, error, completed_at).
	Save(job *JobRecord) error
	// Load retrieves a single job by ID.
	Load(id uint) (*JobRecord, error)
	// LoadPending returns all jobs in Pending or Running state; used for crash recovery.
	LoadPending() ([]*JobRecord, error)
}

// JobRecord captures the full execution state of a single pipeline job.
type JobRecord struct {
	ID           uint
	PipelineName string
	Status       JobStatus
	// CurrentStep is the index of the step to execute next; used for crash recovery.
	CurrentStep int
	// Input holds the raw business parameters; the Runner does not parse this field.
	Input       json.RawMessage
	Error       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt *time.Time
}

// JobStatus is the lifecycle state of a job.
type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobRunning   JobStatus = "running"
	JobCompleted JobStatus = "completed"
	JobFailed    JobStatus = "failed"
)
