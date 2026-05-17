package database

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/esp32-rss-display/backend/server/models"
	"github.com/esp32-rss-display/backend/server/pipeline"
	"gorm.io/gorm"
)

// GORMJobStore implements pipeline.JobStore using GORM and models.Job.
type GORMJobStore struct {
	db *gorm.DB
}

// NewGORMJobStore creates a GORMJobStore backed by db.
func NewGORMJobStore(db *gorm.DB) *GORMJobStore {
	return &GORMJobStore{db: db}
}

// Create inserts a new job row and populates job.ID on success.
func (s *GORMJobStore) Create(job *pipeline.JobRecord) error {
	m := models.Job{
		PipelineName: job.PipelineName,
		Status:       string(job.Status),
		CurrentStep:  job.CurrentStep,
		Input:        string(job.Input),
		CreatedAt:    job.CreatedAt,
		UpdatedAt:    job.UpdatedAt,
	}
	if err := s.db.Create(&m).Error; err != nil {
		return fmt.Errorf("GORMJobStore.Create: %w", err)
	}
	job.ID = m.ID
	return nil
}

// Save updates the mutable fields of an existing job row.
func (s *GORMJobStore) Save(job *pipeline.JobRecord) error {
	updates := map[string]any{
		"status":       string(job.Status),
		"current_step": job.CurrentStep,
		"error":        job.Error,
		"updated_at":   job.UpdatedAt,
		"completed_at": job.CompletedAt,
	}
	if err := s.db.Model(&models.Job{}).Where("id = ?", job.ID).Updates(updates).Error; err != nil {
		return fmt.Errorf("GORMJobStore.Save: %w", err)
	}
	return nil
}

// Load retrieves a single job by ID.
func (s *GORMJobStore) Load(id uint) (*pipeline.JobRecord, error) {
	var m models.Job
	if err := s.db.First(&m, id).Error; err != nil {
		return nil, fmt.Errorf("GORMJobStore.Load(%d): %w", id, err)
	}
	return modelToRecord(&m), nil
}

// LoadPending returns all jobs with status "pending" or "running".
func (s *GORMJobStore) LoadPending() ([]*pipeline.JobRecord, error) {
	var ms []models.Job
	if err := s.db.Where("status IN ?", []string{
		string(pipeline.JobPending),
		string(pipeline.JobRunning),
	}).Find(&ms).Error; err != nil {
		return nil, fmt.Errorf("GORMJobStore.LoadPending: %w", err)
	}
	records := make([]*pipeline.JobRecord, len(ms))
	for i, m := range ms {
		records[i] = modelToRecord(&m)
	}
	return records, nil
}

// SetDeviceID associates a device with a job; used by HTTP handlers after Submit.
func (s *GORMJobStore) SetDeviceID(jobID uint, deviceID string) error {
	if err := s.db.Model(&models.Job{}).Where("id = ?", jobID).
		Updates(map[string]any{
			"device_id":  deviceID,
			"updated_at": time.Now(),
		}).Error; err != nil {
		return fmt.Errorf("GORMJobStore.SetDeviceID: %w", err)
	}
	return nil
}

func modelToRecord(m *models.Job) *pipeline.JobRecord {
	rec := &pipeline.JobRecord{
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
	return rec
}
