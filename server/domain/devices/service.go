package devices

import (
	"context"
	"errors"
	"time"

	"github.com/esp32-rss-display/backend/server/models"
	"gorm.io/gorm"
)

var ErrNotFound = gorm.ErrRecordNotFound

type Service interface {
	GetOrCreate(ctx context.Context, deviceID string) (*models.Device, error)
	UpdatePreference(ctx context.Context, deviceID, role, preference string) (*models.Device, error)
	UpdateCurrentItem(ctx context.Context, deviceID string, itemID uint) error
	TouchLastSeen(ctx context.Context, deviceID string) error
}

type serviceImpl struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &serviceImpl{repo: repo}
}

func (s *serviceImpl) GetOrCreate(ctx context.Context, deviceID string) (*models.Device, error) {
	device, err := s.repo.FindByDeviceID(ctx, deviceID)
	if err == nil {
		return device, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	device = newDevice(deviceID)
	if err := s.repo.Create(ctx, device); err != nil {
		return nil, err
	}
	return device, nil
}

func (s *serviceImpl) UpdatePreference(ctx context.Context, deviceID, role, preference string) (*models.Device, error) {
	device, err := s.repo.FindByDeviceID(ctx, deviceID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		device = newDevice(deviceID)
		device.Role = role
		device.Preference = preference
		if err := s.repo.Create(ctx, device); err != nil {
			return nil, err
		}
		return device, nil
	}
	if err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if role != "" {
		updates["role"] = role
	}
	if preference != "" {
		updates["preference"] = preference
	}
	if len(updates) > 0 {
		if err := s.repo.UpdateFields(ctx, deviceID, updates); err != nil {
			return nil, err
		}
	}
	return s.repo.FindByDeviceID(ctx, deviceID)
}

func (s *serviceImpl) UpdateCurrentItem(ctx context.Context, deviceID string, itemID uint) error {
	return s.repo.UpdateFields(ctx, deviceID, map[string]any{
		"current_item_id": itemID,
	})
}

func (s *serviceImpl) TouchLastSeen(ctx context.Context, deviceID string) error {
	return s.repo.UpdateFields(ctx, deviceID, map[string]any{
		"last_seen": time.Now(),
	})
}