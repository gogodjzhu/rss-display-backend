package feeds_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/esp32-rss-display/backend/server/config"
	"github.com/esp32-rss-display/backend/server/domain/feeds"
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
	if err := db.AutoMigrate(&models.Feed{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestInitFeedsCreatesMissingFeeds(t *testing.T) {
	db := newTestDB(t)
	svc := feeds.NewService(feeds.NewGORMRepository())

	configs := []config.FeedConfig{
		{Name: "Feed A", URL: "http://a.example.com/rss", Enabled: true},
		{Name: "Feed B", URL: "http://b.example.com/rss", Enabled: true},
	}
	if err := svc.InitFeeds(context.Background(), db, configs); err != nil {
		t.Fatalf("InitFeeds: %v", err)
	}

	var count int64
	db.Model(&models.Feed{}).Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 feeds, got %d", count)
	}
}

func TestInitFeedsUpdatesExistingFeed(t *testing.T) {
	db := newTestDB(t)
	svc := feeds.NewService(feeds.NewGORMRepository())

	existing := models.Feed{Name: "Old Name", URL: "http://a.example.com/rss", Enabled: false}
	db.Create(&existing)

	configs := []config.FeedConfig{
		{Name: "New Name", URL: "http://a.example.com/rss", Enabled: true},
	}
	if err := svc.InitFeeds(context.Background(), db, configs); err != nil {
		t.Fatalf("InitFeeds: %v", err)
	}

	var feed models.Feed
	db.Where("url = ?", "http://a.example.com/rss").First(&feed)
	if feed.Name != "New Name" {
		t.Fatalf("expected name %q, got %q", "New Name", feed.Name)
	}
	if !feed.Enabled {
		t.Fatal("expected feed to be enabled")
	}
}

func TestInitFeedsDisablesStaleFeeds(t *testing.T) {
	db := newTestDB(t)
	svc := feeds.NewService(feeds.NewGORMRepository())

	stale := models.Feed{Name: "Stale", URL: "http://stale.example.com/rss", Enabled: true}
	db.Create(&stale)

	// No configs — stale should be disabled.
	if err := svc.InitFeeds(context.Background(), db, nil); err != nil {
		t.Fatalf("InitFeeds: %v", err)
	}

	var feed models.Feed
	db.First(&feed, stale.ID)
	if feed.Enabled {
		t.Fatal("expected stale feed to be disabled")
	}
}

func TestInitFeedsIdempotent(t *testing.T) {
	db := newTestDB(t)
	svc := feeds.NewService(feeds.NewGORMRepository())

	configs := []config.FeedConfig{
		{Name: "Feed A", URL: "http://a.example.com/rss", Enabled: true},
	}
	for range 3 {
		if err := svc.InitFeeds(context.Background(), db, configs); err != nil {
			t.Fatalf("InitFeeds: %v", err)
		}
	}

	var count int64
	db.Model(&models.Feed{}).Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 feed after idempotent runs, got %d", count)
	}
}
