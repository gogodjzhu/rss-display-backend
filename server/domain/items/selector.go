package items

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/esp32-rss-display/backend/server/models"
)

const PlaceholderItemID uint = 0
const PlaceholderTitle = "Working on it..."

var PlaceholderItem = &models.Item{
	ID:    PlaceholderItemID,
	Title: PlaceholderTitle,
}

type ItemSelector interface {
	Select(ctx context.Context, device models.Device) (*models.Item, error)
}

type WeightedItemSelector struct {
	repo        Repository
	now         func() time.Time
	randFloat64 func() float64
}

func NewWeightedItemSelector(repo Repository) *WeightedItemSelector {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	return &WeightedItemSelector{
		repo:        repo,
		now:         time.Now,
		randFloat64: rng.Float64,
	}
}

func NewSeededItemSelector(repo Repository, now time.Time, seed int64) *WeightedItemSelector {
	rng := rand.New(rand.NewSource(seed))
	return &WeightedItemSelector{
		repo:        repo,
		now:         func() time.Time { return now },
		randFloat64: rng.Float64,
	}
}

func (s *WeightedItemSelector) Select(ctx context.Context, device models.Device) (*models.Item, error) {
	allItems, err := s.repo.ListEnabled(ctx)
	if err != nil {
		return nil, err
	}

	if len(allItems) == 0 {
		return PlaceholderItem, nil
	}

	if len(allItems) == 1 {
		return &allItems[0], nil
	}

	now := s.now()
	weights := make([]float64, len(allItems))
	totalWeight := 0.0

	for i, item := range allItems {
		weight := s.itemWeight(item, device.CurrentItemID, now)
		weights[i] = weight
		totalWeight += weight
	}

	if totalWeight == 0 {
		for i, item := range allItems {
			if device.CurrentItemID != nil && item.ID == *device.CurrentItemID {
				continue
			}
			weights[i] = 1
			totalWeight += 1
		}
	}

	if totalWeight == 0 {
		return &allItems[0], nil
	}

	target := s.randFloat64() * totalWeight
	running := 0.0
	for i := range allItems {
		running += weights[i]
		if target < running {
			return &allItems[i], nil
		}
	}

	return &allItems[len(allItems)-1], nil
}

func (s *WeightedItemSelector) itemWeight(item models.Item, currentItemID *uint, now time.Time) float64 {
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

func freshnessWeight(t time.Time, now time.Time) float64 {
	ageHours := now.Sub(t).Hours()
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
		return 1 + (24*7/(ageHours+24))
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