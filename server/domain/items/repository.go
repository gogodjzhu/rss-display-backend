package items

import (
	"context"
	"errors"

	"github.com/esp32-rss-display/backend/server/models"
	"gorm.io/gorm"
)

// Repository is the data-access contract for items.
type Repository interface {
	FindByID(ctx context.Context, db *gorm.DB, id uint) (*models.Item, error)
	RecordShow(ctx context.Context, db *gorm.DB, deviceID string, itemID uint) error
	RecordRead(ctx context.Context, db *gorm.DB, deviceID string, itemID uint) error
	RecordRating(ctx context.Context, db *gorm.DB, deviceID string, itemID uint, rating int) error
}

// GORMRepository is the GORM-backed implementation of Repository.
type GORMRepository struct{}

func NewGORMRepository() *GORMRepository { return &GORMRepository{} }

func (r *GORMRepository) FindByID(ctx context.Context, db *gorm.DB, id uint) (*models.Item, error) {
	var item models.Item
	if err := db.WithContext(ctx).Select("id").First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}
	return &item, nil
}

func (r *GORMRepository) RecordShow(ctx context.Context, db *gorm.DB, deviceID string, itemID uint) error {
	return db.WithContext(ctx).Create(&models.ItemShow{ItemID: itemID, DeviceID: deviceID}).Error
}

func (r *GORMRepository) RecordRead(ctx context.Context, db *gorm.DB, deviceID string, itemID uint) error {
	return db.WithContext(ctx).Create(&models.ItemRead{ItemID: itemID, DeviceID: deviceID}).Error
}

func (r *GORMRepository) RecordRating(ctx context.Context, db *gorm.DB, deviceID string, itemID uint, rating int) error {
	return db.WithContext(ctx).Create(&models.ItemRating{
		ItemID:   itemID,
		DeviceID: deviceID,
		Rating:   rating,
	}).Error
}
