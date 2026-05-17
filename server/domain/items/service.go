package items

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// Service is the business-logic contract for item interactions.
type Service interface {
	RecordShow(ctx context.Context, db *gorm.DB, deviceID string, itemID uint) error
	RecordRead(ctx context.Context, db *gorm.DB, deviceID string, itemID uint) error
	UpdateRating(ctx context.Context, db *gorm.DB, deviceID string, itemID uint, rating int) error
}

type serviceImpl struct {
	repo Repository
}

// NewService creates a Service backed by the given repository.
func NewService(repo Repository) Service {
	return &serviceImpl{repo: repo}
}

func (s *serviceImpl) RecordShow(ctx context.Context, db *gorm.DB, deviceID string, itemID uint) error {
	return s.repo.RecordShow(ctx, db, deviceID, itemID)
}

func (s *serviceImpl) RecordRead(ctx context.Context, db *gorm.DB, deviceID string, itemID uint) error {
	return s.repo.RecordRead(ctx, db, deviceID, itemID)
}

func (s *serviceImpl) UpdateRating(ctx context.Context, db *gorm.DB, deviceID string, itemID uint, rating int) error {
	if rating < 1 || rating > 5 {
		return fmt.Errorf("rating must be between 1 and 5, got %d", rating)
	}
	if _, err := s.repo.FindByID(ctx, db, itemID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return gorm.ErrRecordNotFound
		}
		return err
	}
	return s.repo.RecordRating(ctx, db, deviceID, itemID, rating)
}
