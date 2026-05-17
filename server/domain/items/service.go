package items

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/esp32-rss-display/backend/server/models"
	"gorm.io/gorm"
)

var ErrNotFound = gorm.ErrRecordNotFound

type Service interface {
	RecordShow(ctx context.Context, deviceID string, itemID uint) error
	RecordRead(ctx context.Context, deviceID string, itemID uint) error
	UpdateRating(ctx context.Context, deviceID string, itemID uint, rating int) error
	FindByID(ctx context.Context, id uint) (*models.Item, error)
	FindByIDFull(ctx context.Context, id uint) (*models.Item, error)
	FindByTimeRange(ctx context.Context, start, end time.Time) ([]models.Item, error)
	FindByIDs(ctx context.Context, ids []uint) ([]models.Item, error)
	ListEnabled(ctx context.Context) ([]models.Item, error)
	CreateIfNew(ctx context.Context, item *models.Item) (created bool, err error)
	UpdateContent(ctx context.Context, id uint, content string) error
	UpdateAbstract(ctx context.Context, id uint, abstract string) error
}

type serviceImpl struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &serviceImpl{repo: repo}
}

func (s *serviceImpl) RecordShow(ctx context.Context, deviceID string, itemID uint) error {
	return s.repo.RecordShow(ctx, deviceID, itemID)
}

func (s *serviceImpl) RecordRead(ctx context.Context, deviceID string, itemID uint) error {
	return s.repo.RecordRead(ctx, deviceID, itemID)
}

func (s *serviceImpl) UpdateRating(ctx context.Context, deviceID string, itemID uint, rating int) error {
	if rating < 1 || rating > 5 {
		return fmt.Errorf("rating must be between 1 and 5, got %d", rating)
	}
	if _, err := s.repo.FindByID(ctx, itemID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return gorm.ErrRecordNotFound
		}
		return err
	}
	return s.repo.RecordRating(ctx, deviceID, itemID, rating)
}

func (s *serviceImpl) FindByID(ctx context.Context, id uint) (*models.Item, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *serviceImpl) FindByIDFull(ctx context.Context, id uint) (*models.Item, error) {
	return s.repo.FindByIDFull(ctx, id)
}

func (s *serviceImpl) FindByTimeRange(ctx context.Context, start, end time.Time) ([]models.Item, error) {
	return s.repo.FindByTimeRange(ctx, start, end)
}

func (s *serviceImpl) FindByIDs(ctx context.Context, ids []uint) ([]models.Item, error) {
	return s.repo.FindByIDs(ctx, ids)
}

func (s *serviceImpl) ListEnabled(ctx context.Context) ([]models.Item, error) {
	return s.repo.ListEnabled(ctx)
}

func (s *serviceImpl) CreateIfNew(ctx context.Context, item *models.Item) (bool, error) {
	return s.repo.CreateIfNew(ctx, item)
}

func (s *serviceImpl) UpdateContent(ctx context.Context, id uint, content string) error {
	return s.repo.UpdateContent(ctx, id, content)
}

func (s *serviceImpl) UpdateAbstract(ctx context.Context, id uint, abstract string) error {
	return s.repo.UpdateAbstract(ctx, id, abstract)
}