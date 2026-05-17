package items_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/esp32-rss-display/backend/server/domain/items"
	"github.com/esp32-rss-display/backend/server/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Feed{}, &models.Item{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func createFeed(t *testing.T, db *gorm.DB) models.Feed {
	t.Helper()
	feed := models.Feed{Name: "test", URL: "http://test.example.com", Enabled: true}
	if err := db.Create(&feed).Error; err != nil {
		t.Fatalf("create feed: %v", err)
	}
	return feed
}

func createItem(t *testing.T, db *gorm.DB, feedID uint, title string, published time.Time) models.Item {
	t.Helper()
	item := models.Item{FeedID: feedID, Title: title, URL: "http://example.com/" + title, PublishedAt: &published}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("create item: %v", err)
	}
	return item
}

func TestWeightedItemSelectorReturnsPlaceholderWhenNoItems(t *testing.T) {
	db := newTestDB(t)
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	sel := items.NewSeededItemSelector(items.NewGORMRepository(db), now, 1)

	item, err := sel.Select(context.Background(), models.Device{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.ID != items.PlaceholderItemID {
		t.Fatalf("expected placeholder id %d, got %d", items.PlaceholderItemID, item.ID)
	}
	if item.Title != items.PlaceholderTitle {
		t.Fatalf("expected placeholder title %q, got %q", items.PlaceholderTitle, item.Title)
	}
}

func TestWeightedItemSelectorPrefersRecentItems(t *testing.T) {
	db := newTestDB(t)
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	feed := createFeed(t, db)

	old := createItem(t, db, feed.ID, "old", now.Add(-30*24*time.Hour))
	mid := createItem(t, db, feed.ID, "mid", now.Add(-48*time.Hour))
	newest := createItem(t, db, feed.ID, "new", now.Add(-2*time.Hour))

	sel := items.NewSeededItemSelector(items.NewGORMRepository(db), now, 42)
	counts := map[uint]int{}
	for range 4000 {
		item, err := sel.Select(context.Background(), models.Device{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		counts[item.ID]++
	}

	if counts[newest.ID] <= counts[mid.ID] {
		t.Fatalf("expected newest to dominate: newest=%d mid=%d", counts[newest.ID], counts[mid.ID])
	}
	if counts[mid.ID] <= counts[old.ID] {
		t.Fatalf("expected mid to beat oldest: mid=%d old=%d", counts[mid.ID], counts[old.ID])
	}
}

func TestWeightedItemSelectorAvoidsCurrentItem(t *testing.T) {
	db := newTestDB(t)
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	feed := createFeed(t, db)

	itemA := createItem(t, db, feed.ID, "a", now.Add(-time.Hour))
	_ = createItem(t, db, feed.ID, "b", now.Add(-time.Hour))

	sel := items.NewSeededItemSelector(items.NewGORMRepository(db), now, 99)
	device := models.Device{CurrentItemID: &itemA.ID}
	for range 200 {
		got, err := sel.Select(context.Background(), device)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID == itemA.ID {
			t.Fatalf("selector returned current item")
		}
	}
}

func TestWeightedItemSelectorReturnsSingleItem(t *testing.T) {
	db := newTestDB(t)
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	feed := createFeed(t, db)
	item := createItem(t, db, feed.ID, "only", now.Add(-time.Hour))

	sel := items.NewSeededItemSelector(items.NewGORMRepository(db), now, 1)
	got, err := sel.Select(context.Background(), models.Device{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != item.ID {
		t.Fatalf("expected item %d, got %d", item.ID, got.ID)
	}
}

func TestPlaceholderItem(t *testing.T) {
	if items.PlaceholderItem == nil {
		t.Fatal("PlaceholderItem should not be nil")
	}
	if items.PlaceholderItem.ID != items.PlaceholderItemID {
		t.Fatalf("PlaceholderItem.ID mismatch")
	}
	if items.PlaceholderItem.Title != items.PlaceholderTitle {
		t.Fatalf("PlaceholderItem.Title mismatch")
	}
}