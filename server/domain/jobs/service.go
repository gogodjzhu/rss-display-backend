package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/esp32-rss-display/backend/server/models"
	"github.com/esp32-rss-display/backend/server/pipeline"
	"gorm.io/gorm"
)

type Repository interface {
	Create(ctx context.Context, job *models.Job) error
	FindByDeviceIDLatest(ctx context.Context, deviceID string) (*models.Job, error)
	FindByIDAndDevice(ctx context.Context, jobID uint, deviceID string) (*models.Job, error)
	UpdateDeviceID(ctx context.Context, jobID uint, deviceID string) error
	UpdateReport(ctx context.Context, jobID uint, report string, level2IDs string) error
	FindByID(ctx context.Context, jobID uint) (*models.Job, error)
}

type GORMRepository struct {
	db *gorm.DB
}

func NewGORMRepository(db *gorm.DB) *GORMRepository {
	return &GORMRepository{db: db}
}

func (r *GORMRepository) Create(ctx context.Context, job *models.Job) error {
	return r.db.WithContext(ctx).Create(job).Error
}

func (r *GORMRepository) FindByDeviceIDLatest(ctx context.Context, deviceID string) (*models.Job, error) {
	var job models.Job
	if err := r.db.WithContext(ctx).Where("device_id = ?", deviceID).
		Order("created_at DESC, id DESC").First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}
	return &job, nil
}

func (r *GORMRepository) FindByIDAndDevice(ctx context.Context, jobID uint, deviceID string) (*models.Job, error) {
	var job models.Job
	if err := r.db.WithContext(ctx).Where("id = ? AND device_id = ?", jobID, deviceID).First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}
	return &job, nil
}

func (r *GORMRepository) UpdateDeviceID(ctx context.Context, jobID uint, deviceID string) error {
	return r.db.WithContext(ctx).Model(&models.Job{}).Where("id = ?", jobID).
		Updates(map[string]any{
			"device_id":  deviceID,
			"updated_at": time.Now(),
		}).Error
}

func (r *GORMRepository) UpdateReport(ctx context.Context, jobID uint, report string, level2IDs string) error {
	return r.db.WithContext(ctx).Model(&models.Job{}).Where("id = ?", jobID).
		Updates(map[string]any{
			"report":      report,
			"level2_ids":  level2IDs,
			"updated_at":  time.Now(),
		}).Error
}

func (r *GORMRepository) FindByID(ctx context.Context, jobID uint) (*models.Job, error) {
	var job models.Job
	if err := r.db.WithContext(ctx).First(&job, jobID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}
	return &job, nil
}

type Service interface {
	CreateJob(ctx context.Context, deviceID string, timeStart, timeEnd time.Time) (*models.Job, error)
	GetLatestJob(ctx context.Context, deviceID string) (*models.Job, error)
	GetJobByID(ctx context.Context, jobID uint, deviceID string) (*models.Job, error)
	AssociateDevice(ctx context.Context, jobID uint, deviceID string) error
	UpdateReport(ctx context.Context, jobID uint, report string, level2IDs []uint) error
}

type serviceImpl struct {
	repo   Repository
	runner *pipeline.Runner
}

func NewService(repo Repository, runner *pipeline.Runner) Service {
	return &serviceImpl{repo: repo, runner: runner}
}

func (s *serviceImpl) CreateJob(ctx context.Context, deviceID string, timeStart, timeEnd time.Time) (*models.Job, error) {
	input, err := json.Marshal(map[string]any{
		"device_id":        deviceID,
		"time_range_start": timeStart,
		"time_range_end":   timeEnd,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal job input: %w", err)
	}

	rec, err := s.runner.Submit(ctx, "rss_pipeline", input)
	if err != nil {
		return nil, fmt.Errorf("submit pipeline job: %w", err)
	}

	if err := s.repo.UpdateDeviceID(ctx, rec.ID, deviceID); err != nil {
		_ = err
	}

	return s.repo.FindByID(ctx, rec.ID)
}

func (s *serviceImpl) GetLatestJob(ctx context.Context, deviceID string) (*models.Job, error) {
	return s.repo.FindByDeviceIDLatest(ctx, deviceID)
}

func (s *serviceImpl) GetJobByID(ctx context.Context, jobID uint, deviceID string) (*models.Job, error) {
	return s.repo.FindByIDAndDevice(ctx, jobID, deviceID)
}

func (s *serviceImpl) AssociateDevice(ctx context.Context, jobID uint, deviceID string) error {
	return s.repo.UpdateDeviceID(ctx, jobID, deviceID)
}

func (s *serviceImpl) UpdateReport(ctx context.Context, jobID uint, report string, level2IDs []uint) error {
	idsJSON := marshalIDs(level2IDs)
	return s.repo.UpdateReport(ctx, jobID, report, idsJSON)
}

func marshalIDs(ids []uint) string {
	if len(ids) == 0 {
		return ""
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		b, _ := json.Marshal(id)
		parts[i] = string(b)
	}
	return "[" + joinStrings(parts, ",") + "]"
}

func joinStrings(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}