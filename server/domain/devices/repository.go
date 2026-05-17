package devices

import (
	"context"
	"errors"
	"time"

	"github.com/esp32-rss-display/backend/server/models"
	"gorm.io/gorm"
)

// Repository is the data-access contract for devices.
type Repository interface {
	FindByDeviceID(ctx context.Context, db *gorm.DB, deviceID string) (*models.Device, error)
	Create(ctx context.Context, db *gorm.DB, device *models.Device) error
	Save(ctx context.Context, db *gorm.DB, device *models.Device) error
	FindLatestJob(ctx context.Context, db *gorm.DB, deviceID string) (*models.Job, error)
	FindJobByID(ctx context.Context, db *gorm.DB, jobID uint, deviceID string) (*models.Job, error)
}

// GORMRepository is the GORM-backed implementation of Repository.
type GORMRepository struct{}

func NewGORMRepository() *GORMRepository { return &GORMRepository{} }

func (r *GORMRepository) FindByDeviceID(ctx context.Context, db *gorm.DB, deviceID string) (*models.Device, error) {
	var device models.Device
	if err := db.WithContext(ctx).Where("device_id = ?", deviceID).First(&device).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}
	return &device, nil
}

func (r *GORMRepository) Create(ctx context.Context, db *gorm.DB, device *models.Device) error {
	return db.WithContext(ctx).Create(device).Error
}

func (r *GORMRepository) Save(ctx context.Context, db *gorm.DB, device *models.Device) error {
	return db.WithContext(ctx).Save(device).Error
}

func (r *GORMRepository) FindLatestJob(ctx context.Context, db *gorm.DB, deviceID string) (*models.Job, error) {
	var job models.Job
	if err := db.WithContext(ctx).Where("device_id = ?", deviceID).
		Order("created_at DESC, id DESC").First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}
	return &job, nil
}

func (r *GORMRepository) FindJobByID(ctx context.Context, db *gorm.DB, jobID uint, deviceID string) (*models.Job, error) {
	var job models.Job
	if err := db.WithContext(ctx).Where("id = ? AND device_id = ?", jobID, deviceID).First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}
	return &job, nil
}

// newDevice creates a default Device value for a newly-seen device ID.
func newDevice(deviceID string) *models.Device {
	now := time.Now()
	return &models.Device{
		DeviceID:  deviceID,
		CreatedAt: now,
		LastSeen:  now,
	}
}
