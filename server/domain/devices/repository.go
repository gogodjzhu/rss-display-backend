package devices

import (
	"context"
	"errors"
	"time"

	"github.com/esp32-rss-display/backend/server/models"
	"gorm.io/gorm"
)

type Repository interface {
	FindByDeviceID(ctx context.Context, deviceID string) (*models.Device, error)
	Create(ctx context.Context, device *models.Device) error
	Save(ctx context.Context, device *models.Device) error
	UpdateFields(ctx context.Context, deviceID string, fields map[string]any) error
	FindLatestJob(ctx context.Context, deviceID string) (*models.Job, error)
	FindJobByID(ctx context.Context, jobID uint, deviceID string) (*models.Job, error)
}

type GORMRepository struct {
	db *gorm.DB
}

func NewGORMRepository(db *gorm.DB) *GORMRepository {
	return &GORMRepository{db: db}
}

func (r *GORMRepository) FindByDeviceID(ctx context.Context, deviceID string) (*models.Device, error) {
	var device models.Device
	if err := r.db.WithContext(ctx).Where("device_id = ?", deviceID).First(&device).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}
	return &device, nil
}

func (r *GORMRepository) Create(ctx context.Context, device *models.Device) error {
	return r.db.WithContext(ctx).Create(device).Error
}

func (r *GORMRepository) Save(ctx context.Context, device *models.Device) error {
	return r.db.WithContext(ctx).Save(device).Error
}

func (r *GORMRepository) UpdateFields(ctx context.Context, deviceID string, fields map[string]any) error {
	return r.db.WithContext(ctx).Model(&models.Device{}).Where("device_id = ?", deviceID).Updates(fields).Error
}

func (r *GORMRepository) FindLatestJob(ctx context.Context, deviceID string) (*models.Job, error) {
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

func (r *GORMRepository) FindJobByID(ctx context.Context, jobID uint, deviceID string) (*models.Job, error) {
	var job models.Job
	if err := r.db.WithContext(ctx).Where("id = ? AND device_id = ?", jobID, deviceID).First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}
	return &job, nil
}

func newDevice(deviceID string) *models.Device {
	now := time.Now()
	return &models.Device{
		DeviceID:  deviceID,
		CreatedAt: now,
		LastSeen:  now,
	}
}