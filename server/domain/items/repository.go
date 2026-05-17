package items

import (
	"context"
	"errors"
	"time"

	"github.com/esp32-rss-display/backend/server/models"
	"gorm.io/gorm"
)

type Repository interface {
	FindByID(ctx context.Context, id uint) (*models.Item, error)
	FindByIDFull(ctx context.Context, id uint) (*models.Item, error)
	FindByTimeRange(ctx context.Context, start, end time.Time) ([]models.Item, error)
	FindByIDs(ctx context.Context, ids []uint) ([]models.Item, error)
	ListEnabled(ctx context.Context) ([]models.Item, error)
	CreateIfNew(ctx context.Context, item *models.Item) (created bool, err error)
	UpdateContent(ctx context.Context, id uint, content string) error
	UpdateAbstract(ctx context.Context, id uint, abstract string) error
	RecordShow(ctx context.Context, deviceID string, itemID uint) error
	RecordRead(ctx context.Context, deviceID string, itemID uint) error
	RecordRating(ctx context.Context, deviceID string, itemID uint, rating int) error
}

type GORMRepository struct {
	db *gorm.DB
}

func NewGORMRepository(db *gorm.DB) *GORMRepository {
	return &GORMRepository{db: db}
}

func (r *GORMRepository) FindByID(ctx context.Context, id uint) (*models.Item, error) {
	var item models.Item
	if err := r.db.WithContext(ctx).Select("id").First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}
	return &item, nil
}

func (r *GORMRepository) FindByIDFull(ctx context.Context, id uint) (*models.Item, error) {
	var item models.Item
	if err := r.db.WithContext(ctx).First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}
	return &item, nil
}

func (r *GORMRepository) FindByTimeRange(ctx context.Context, start, end time.Time) ([]models.Item, error) {
	var items []models.Item
	err := r.db.WithContext(ctx).Where(
		"(published_at BETWEEN ? AND ?) OR (published_at IS NULL AND created_at BETWEEN ? AND ?)",
		start, end, start, end,
	).Find(&items).Error
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (r *GORMRepository) FindByIDs(ctx context.Context, ids []uint) ([]models.Item, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var items []models.Item
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *GORMRepository) ListEnabled(ctx context.Context) ([]models.Item, error) {
	var items []models.Item
	err := r.db.WithContext(ctx).
		Joins("JOIN feeds ON feeds.id = items.feed_id").
		Where("feeds.enabled = ?", true).
		Order("COALESCE(items.published_at, items.created_at) DESC, items.id DESC").
		Find(&items).Error
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (r *GORMRepository) CreateIfNew(ctx context.Context, item *models.Item) (bool, error) {
	var existing models.Item
	err := r.db.WithContext(ctx).Where("feed_id = ? AND url = ?", item.FeedID, item.URL).First(&existing).Error
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	if err := r.db.WithContext(ctx).Create(item).Error; err != nil {
		return false, err
	}
	return true, nil
}

func (r *GORMRepository) UpdateContent(ctx context.Context, id uint, content string) error {
	return r.db.WithContext(ctx).Model(&models.Item{}).Where("id = ?", id).Update("content", content).Error
}

func (r *GORMRepository) UpdateAbstract(ctx context.Context, id uint, abstract string) error {
	return r.db.WithContext(ctx).Model(&models.Item{}).Where("id = ?", id).Update("abstract", abstract).Error
}

func (r *GORMRepository) RecordShow(ctx context.Context, deviceID string, itemID uint) error {
	return r.db.WithContext(ctx).Create(&models.ItemShow{ItemID: itemID, DeviceID: deviceID}).Error
}

func (r *GORMRepository) RecordRead(ctx context.Context, deviceID string, itemID uint) error {
	return r.db.WithContext(ctx).Create(&models.ItemRead{ItemID: itemID, DeviceID: deviceID}).Error
}

func (r *GORMRepository) RecordRating(ctx context.Context, deviceID string, itemID uint, rating int) error {
	return r.db.WithContext(ctx).Create(&models.ItemRating{
		ItemID:   itemID,
		DeviceID: deviceID,
		Rating:   rating,
	}).Error
}