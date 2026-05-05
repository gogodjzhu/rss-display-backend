package api

import (
	"context"
	"math/rand"
	"path/filepath"
	"testing"
	"time"

	"github.com/esp32-rss-display/backend/server/database"
	"github.com/esp32-rss-display/backend/server/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestWeightedNextItemSelectorReturnsPlaceholderWhenNoItems(t *testing.T) {
	db := newTestDB(t)
	selector := newSeededSelector(time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC), 1)

	item, err := selector.SelectNext(context.Background(), db, models.Device{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if item.ID != PlaceholderItemID {
		t.Fatalf("expected placeholder item id %d, got %d", PlaceholderItemID, item.ID)
	}
	if item.Title != PlaceholderTitle {
		t.Fatalf("expected placeholder title %q, got %q", PlaceholderTitle, item.Title)
	}
}

func TestWeightedNextItemSelectorPrefersRecentItems(t *testing.T) {
	db := newTestDB(t)
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	feed := createTestFeed(t, db, "news", true)

	old := createTestItem(t, db, feed.ID, "old", now.Add(-30*24*time.Hour), nil)
	mid := createTestItem(t, db, feed.ID, "mid", now.Add(-48*time.Hour), nil)
	newest := createTestItem(t, db, feed.ID, "new", now.Add(-2*time.Hour), nil)

	selector := newSeededSelector(now, 42)
	counts := map[uint]int{}
	for range 4000 {
		item, err := selector.SelectNext(context.Background(), db, models.Device{})
		if err != nil {
			t.Fatalf("SelectNext returned error: %v", err)
		}
		counts[item.ID]++
	}

	if counts[newest.ID] <= counts[mid.ID] {
		t.Fatalf("expected newest item to be selected most often, got newest=%d mid=%d", counts[newest.ID], counts[mid.ID])
	}
	if counts[mid.ID] <= counts[old.ID] {
		t.Fatalf("expected mid item to beat oldest item, got mid=%d old=%d", counts[mid.ID], counts[old.ID])
	}
}

func TestWeightedNextItemSelectorAvoidsImmediateSequentialItems(t *testing.T) {
	db := newTestDB(t)
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	feed := createTestFeed(t, db, "news", true)

	items := make([]models.Item, 0, 5)
	for range 5 {
		items = append(items, createTestItem(t, db, feed.ID, "item", now.Add(-24*time.Hour), nil))
	}

	selector := newSeededSelector(now, 7)
	device := models.Device{CurrentItemID: &items[2].ID}
	counts := map[uint]int{}

	for range 4000 {
		item, err := selector.SelectNext(context.Background(), db, device)
		if err != nil {
			t.Fatalf("SelectNext returned error: %v", err)
		}
		counts[item.ID]++
		if item.ID == items[2].ID {
			t.Fatalf("selector repeated the current item %d", item.ID)
		}
	}

	if counts[items[3].ID] >= counts[items[4].ID] {
		t.Fatalf("expected immediate next id to be penalized, got next=%d farther=%d", counts[items[3].ID], counts[items[4].ID])
	}
	if counts[items[1].ID] >= counts[items[0].ID] {
		t.Fatalf("expected immediate previous id to be penalized, got previous=%d farther=%d", counts[items[1].ID], counts[items[0].ID])
	}
}

func newSeededSelector(now time.Time, seed int64) *WeightedNextItemSelector {
	rng := rand.New(rand.NewSource(seed))
	return &WeightedNextItemSelector{
		now: func() time.Time {
			return now
		},
		randFloat64: rng.Float64,
	}
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	if err := db.AutoMigrate(&models.Device{}, &models.Feed{}, &models.Item{}, &models.ItemShow{}, &models.ItemRead{}, &models.ItemRating{}); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}

	database.DB = db
	return db
}

func createTestFeed(t *testing.T, db *gorm.DB, name string, enabled bool) models.Feed {
	t.Helper()

	feed := models.Feed{Name: name, URL: name + ".example.com", Enabled: enabled, CreatedAt: time.Now().UTC()}
	if err := db.Create(&feed).Error; err != nil {
		t.Fatalf("failed to create feed: %v", err)
	}

	return feed
}

func createTestItem(t *testing.T, db *gorm.DB, feedID uint, title string, createdAt time.Time, publishedAt *time.Time) models.Item {
	t.Helper()

	item := models.Item{
		FeedID:      feedID,
		Title:       title,
		URL:         "https://" + title + ".example.com",
		ImageURL:    "https://example.com/" + title + ".jpg",
		PublishedAt: publishedAt,
		CreatedAt:   createdAt,
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("failed to create item: %v", err)
	}

	return item
}
