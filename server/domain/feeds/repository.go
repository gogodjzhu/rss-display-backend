package feeds

import (
	"context"
	"errors"

	"github.com/esp32-rss-display/backend/server/models"
	"gorm.io/gorm"
)

type Repository interface {
	FindAll(ctx context.Context) ([]models.Feed, error)
	FindByURL(ctx context.Context, url string) (*models.Feed, error)
	FindByID(ctx context.Context, id uint) (*models.Feed, error)
	Create(ctx context.Context, feed *models.Feed) error
	Update(ctx context.Context, feed *models.Feed) error
}

type GORMRepository struct {
	db *gorm.DB
}

func NewGORMRepository(db *gorm.DB) *GORMRepository {
	return &GORMRepository{db: db}
}

func (r *GORMRepository) FindAll(ctx context.Context) ([]models.Feed, error) {
	var feeds []models.Feed
	return feeds, r.db.WithContext(ctx).Find(&feeds).Error
}

func (r *GORMRepository) FindByURL(ctx context.Context, url string) (*models.Feed, error) {
	var feed models.Feed
	if err := r.db.WithContext(ctx).Where("url = ?", url).First(&feed).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}
	return &feed, nil
}

func (r *GORMRepository) FindByID(ctx context.Context, id uint) (*models.Feed, error) {
	var feed models.Feed
	if err := r.db.WithContext(ctx).First(&feed, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}
	return &feed, nil
}

func (r *GORMRepository) Create(ctx context.Context, feed *models.Feed) error {
	return r.db.WithContext(ctx).Create(feed).Error
}

func (r *GORMRepository) Update(ctx context.Context, feed *models.Feed) error {
	return r.db.WithContext(ctx).Save(feed).Error
}