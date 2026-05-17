package pipeline

import (
	"context"
	"time"
)

// Step is a single executable unit in a Pipeline.
// Implementations fetch input from state, perform their work, and write output to state.
type Step interface {
	// Name returns the unique step identifier, used as the state key for the step's output.
	Name() string
	// Run executes the step. It reads prior step outputs from state and writes its own output.
	Run(ctx context.Context, state StateAccessor) error
	// Config returns timeout and retry configuration for this step.
	Config() StepConfig
}

// StepConfig holds execution parameters for a step.
type StepConfig struct {
	Timeout     time.Duration
	RetryPolicy RetryPolicy
}

// RetryPolicy defines exponential-backoff retry behavior.
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}
