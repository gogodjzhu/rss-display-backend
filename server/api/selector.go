package api

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"time"

	"github.com/esp32-rss-display/backend/server/models"
	"gorm.io/gorm"
)

var ErrNoItemsAvailable = errors.New("no items available")

const PlaceholderItemID uint = 0
const PlaceholderTitle = "正在同步内容，请稍后刷新"

var placeholderItem = models.Item{
	ID:    PlaceholderItemID,
	Title: PlaceholderTitle,
}

type NextItemSelector interface {
	SelectNext(ctx context.Context, db *gorm.DB, device models.Device) (models.Item, error)
}

type WeightedNextItemSelector struct {
	now         func() time.Time
	randFloat64 func() float64
}

func NewWeightedNextItemSelector() *WeightedNextItemSelector {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	return &WeightedNextItemSelector{
		now:         time.Now,
		randFloat64: rng.Float64,
	}
}

func (s *WeightedNextItemSelector) SelectNext(ctx context.Context, db *gorm.DB, device models.Device) (models.Item, error) {
	var items []models.Item
	err := db.WithContext(ctx).
		Joins("JOIN feeds ON feeds.id = items.feed_id").
		Where("feeds.enabled = ?", true).
		Order("COALESCE(items.published_at, items.created_at) DESC, items.id DESC").
		Find(&items).Error
	if err != nil {
		return models.Item{}, err
	}

	if len(items) == 0 {
		return placeholderItem, nil
	}

	if len(items) == 1 {
		return items[0], nil
	}

	now := s.now()
	weights := make([]float64, len(items))
	totalWeight := 0.0

	for i, item := range items {
		weight := s.itemWeight(item, device.CurrentItemID, now)
		weights[i] = weight
		totalWeight += weight
	}

	if totalWeight == 0 {
		for i, item := range items {
			if device.CurrentItemID != nil && item.ID == *device.CurrentItemID {
				continue
			}
			weights[i] = 1
			totalWeight += 1
		}
	}

	if totalWeight == 0 {
		return items[0], nil
	}

	target := s.randFloat64() * totalWeight
	running := 0.0
	for i, item := range items {
		running += weights[i]
		if target < running {
			return item, nil
		}
	}

	return items[len(items)-1], nil
}

func (s *WeightedNextItemSelector) itemWeight(item models.Item, currentItemID *uint, now time.Time) float64 {
	if currentItemID != nil && item.ID == *currentItemID {
		return 0
	}

	weight := freshnessWeight(itemTime(item), now)

	if currentItemID != nil {
		weight *= sequencePenalty(item.ID, *currentItemID)
	}

	if weight > 0 && weight < 0.05 {
		return 0.05
	}

	return weight
}

func itemTime(item models.Item) time.Time {
	if item.PublishedAt != nil {
		return *item.PublishedAt
	}

	return item.CreatedAt
}

func freshnessWeight(itemTime time.Time, now time.Time) float64 {
	ageHours := now.Sub(itemTime).Hours()
	if ageHours < 0 {
		ageHours = 0
	}

	switch {
	case ageHours <= 6:
		return 12
	case ageHours <= 24:
		return 8
	case ageHours <= 72:
		return 5
	case ageHours <= 24*7:
		return 3
	default:
		return 1 + (24 * 7 / (ageHours + 24))
	}
}

func sequencePenalty(itemID, currentItemID uint) float64 {
	distance := math.Abs(float64(itemID) - float64(currentItemID))

	switch {
	case distance <= 1:
		return 0.08
	case distance <= 2:
		return 0.35
	case distance <= 5:
		return 0.75
	default:
		return 1
	}
}
