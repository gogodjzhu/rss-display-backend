package devices

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/esp32-rss-display/backend/server/models"
	"github.com/esp32-rss-display/backend/server/pipeline"
	"github.com/esp32-rss-display/backend/server/pipeline/steps"
	"gorm.io/gorm"
)

// JobService manages pipeline jobs for devices.
type JobService interface {
	CreateJob(ctx context.Context, db *gorm.DB, deviceID string, timeStart, timeEnd time.Time) (*models.Job, error)
	GetLatestJob(ctx context.Context, db *gorm.DB, deviceID string) (*models.Job, error)
	GetJobByID(ctx context.Context, db *gorm.DB, jobID uint, deviceID string) (*models.Job, error)
}

// JobStoreWithDeviceID extends pipeline.JobStore with the ability to tag a job
// with a device ID after submission.
type JobStoreWithDeviceID interface {
	pipeline.JobStore
	SetDeviceID(jobID uint, deviceID string) error
}

type jobServiceImpl struct {
	runner    *pipeline.Runner
	jobStore  JobStoreWithDeviceID
	deviceRepo Repository
}

// NewJobService creates a JobService that submits jobs to runner and persists
// device associations via jobStore.
func NewJobService(runner *pipeline.Runner, jobStore JobStoreWithDeviceID, deviceRepo Repository) JobService {
	return &jobServiceImpl{
		runner:     runner,
		jobStore:   jobStore,
		deviceRepo: deviceRepo,
	}
}

func (s *jobServiceImpl) CreateJob(ctx context.Context, db *gorm.DB, deviceID string, timeStart, timeEnd time.Time) (*models.Job, error) {
	input, err := json.Marshal(steps.RSSJobInput{
		DeviceID:       deviceID,
		TimeRangeStart: timeStart,
		TimeRangeEnd:   timeEnd,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal job input: %w", err)
	}

	rec, err := s.runner.Submit(ctx, steps.PipelineName, input)
	if err != nil {
		return nil, fmt.Errorf("submit pipeline job: %w", err)
	}

	if err := s.jobStore.SetDeviceID(rec.ID, deviceID); err != nil {
		// Non-fatal: the job is running; device association is best-effort.
		_ = err
	}

	var job models.Job
	if err := db.WithContext(ctx).First(&job, rec.ID).Error; err != nil {
		return nil, fmt.Errorf("reload job %d: %w", rec.ID, err)
	}
	return &job, nil
}

func (s *jobServiceImpl) GetLatestJob(ctx context.Context, db *gorm.DB, deviceID string) (*models.Job, error) {
	job, err := s.deviceRepo.FindLatestJob(ctx, db, deviceID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, gorm.ErrRecordNotFound
	}
	return job, err
}

func (s *jobServiceImpl) GetJobByID(ctx context.Context, db *gorm.DB, jobID uint, deviceID string) (*models.Job, error) {
	job, err := s.deviceRepo.FindJobByID(ctx, db, jobID, deviceID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, gorm.ErrRecordNotFound
	}
	return job, err
}
