package devices

import (
	"context"
	"errors"

	"github.com/esp32-rss-display/backend/server/models"
	"gorm.io/gorm"
)

// Service is the business-logic contract for device management.
type Service interface {
	// GetOrCreate returns the device for deviceID, creating it if it does not exist.
	GetOrCreate(ctx context.Context, db *gorm.DB, deviceID string) (*models.Device, error)
	// UpdatePreference sets role and preference for the device (creating it if needed).
	UpdatePreference(ctx context.Context, db *gorm.DB, deviceID, role, preference string) (*models.Device, error)
}

type serviceImpl struct {
	repo Repository
}

// NewService creates a Service backed by the given repository.
func NewService(repo Repository) Service {
	return &serviceImpl{repo: repo}
}

func (s *serviceImpl) GetOrCreate(ctx context.Context, db *gorm.DB, deviceID string) (*models.Device, error) {
	device, err := s.repo.FindByDeviceID(ctx, db, deviceID)
	if err == nil {
		return device, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	device = newDevice(deviceID)
	if err := s.repo.Create(ctx, db, device); err != nil {
		return nil, err
	}
	return device, nil
}

func (s *serviceImpl) UpdatePreference(ctx context.Context, db *gorm.DB, deviceID, role, preference string) (*models.Device, error) {
	device, err := s.repo.FindByDeviceID(ctx, db, deviceID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		device = newDevice(deviceID)
		device.Role = role
		device.Preference = preference
		if err := s.repo.Create(ctx, db, device); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	} else {
		updates := map[string]any{}
		if role != "" {
			updates["role"] = role
		}
		if preference != "" {
			updates["preference"] = preference
		}
		if len(updates) > 0 {
			if err := db.WithContext(ctx).Model(device).Updates(updates).Error; err != nil {
				return nil, err
			}
		}
		// Reload after update.
		if err := db.WithContext(ctx).Where("device_id = ?", deviceID).First(device).Error; err != nil {
			return nil, err
		}
	}
	return device, nil
}
