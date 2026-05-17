package feeds

import (
	"context"
	"errors"

	"github.com/esp32-rss-display/backend/server/models"
	"gorm.io/gorm"
)

// Repository is the data-access contract for feeds.
type Repository interface {
	FindAll(ctx context.Context, db *gorm.DB) ([]models.Feed, error)
	FindByURL(ctx context.Context, db *gorm.DB, url string) (*models.Feed, error)
	Create(ctx context.Context, db *gorm.DB, feed *models.Feed) error
	Update(ctx context.Context, db *gorm.DB, feed *models.Feed) error
}

// GORMRepository is the GORM-backed implementation of Repository.
type GORMRepository struct{}

func NewGORMRepository() *GORMRepository { return &GORMRepository{} }

func (r *GORMRepository) FindAll(ctx context.Context, db *gorm.DB) ([]models.Feed, error) {
	var feeds []models.Feed
	return feeds, db.WithContext(ctx).Find(&feeds).Error
}

func (r *GORMRepository) FindByURL(ctx context.Context, db *gorm.DB, url string) (*models.Feed, error) {
	var feed models.Feed
	if err := db.WithContext(ctx).Where("url = ?", url).First(&feed).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}
	return &feed, nil
}

func (r *GORMRepository) Create(ctx context.Context, db *gorm.DB, feed *models.Feed) error {
	return db.WithContext(ctx).Create(feed).Error
}

func (r *GORMRepository) Update(ctx context.Context, db *gorm.DB, feed *models.Feed) error {
	return db.WithContext(ctx).Save(feed).Error
}
