package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/esp32-rss-display/backend/server/models"
	"github.com/esp32-rss-display/backend/server/pipeline"
)

// GORMJobStoreAdapter implements pipeline.JobStore by delegating to the
// GORM-based persistence layer through the GORMRepository.
// This replaces the former database.GORMJobStore.
type GORMJobStoreAdapter struct {
	repo *GORMRepository
}

// NewGORMJobStoreAdapter creates a JobStore-over-GORM adapter.
func NewGORMJobStoreAdapter(repo *GORMRepository) *GORMJobStoreAdapter {
	return &GORMJobStoreAdapter{repo: repo}
}

func (a *GORMJobStoreAdapter) Create(job *pipeline.JobRecord) error {
	ctx := context.Background()
	m := models.Job{
		PipelineName: job.PipelineName,
		Status:       string(job.Status),
		CurrentStep:  job.CurrentStep,
		Input:        string(job.Input),
		CreatedAt:    job.CreatedAt,
		UpdatedAt:    job.UpdatedAt,
	}
	if err := a.repo.db.WithContext(ctx).Create(&m).Error; err != nil {
		return fmt.Errorf("GORMJobStore.Create: %w", err)
	}
	job.ID = m.ID
	return nil
}

func (a *GORMJobStoreAdapter) Save(job *pipeline.JobRecord) error {
	ctx := context.Background()
	updates := map[string]any{
		"status":       string(job.Status),
		"current_step": job.CurrentStep,
		"error":        job.Error,
		"updated_at":   job.UpdatedAt,
		"completed_at": job.CompletedAt,
	}
	return a.repo.db.WithContext(ctx).Model(&models.Job{}).Where("id = ?", job.ID).Updates(updates).Error
}

func (a *GORMJobStoreAdapter) Load(id uint) (*pipeline.JobRecord, error) {
	ctx := context.Background()
	var m models.Job
	if err := a.repo.db.WithContext(ctx).First(&m, id).Error; err != nil {
		return nil, fmt.Errorf("GORMJobStore.Load(%d): %w", id, err)
	}
	return recordFromModel(&m), nil
}

func (a *GORMJobStoreAdapter) LoadPending() ([]*pipeline.JobRecord, error) {
	ctx := context.Background()
	var ms []models.Job
	if err := a.repo.db.WithContext(ctx).Where("status IN ?", []string{
		string(pipeline.JobPending),
		string(pipeline.JobRunning),
	}).Find(&ms).Error; err != nil {
		return nil, fmt.Errorf("GORMJobStore.LoadPending: %w", err)
	}
	records := make([]*pipeline.JobRecord, len(ms))
	for i, m := range ms {
		records[i] = recordFromModel(&m)
	}
	return records, nil
}

func (a *GORMJobStoreAdapter) SetDeviceID(jobID uint, deviceID string) error {
	return a.repo.UpdateDeviceID(context.Background(), jobID, deviceID)
}

func recordFromModel(m *models.Job) *pipeline.JobRecord {
	return &pipeline.JobRecord{
		ID:           m.ID,
		PipelineName: m.PipelineName,
		Status:       pipeline.JobStatus(m.Status),
		CurrentStep:  m.CurrentStep,
		Input:        json.RawMessage(m.Input),
		Error:        m.Error,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
		CompletedAt:  m.CompletedAt,
	}
}