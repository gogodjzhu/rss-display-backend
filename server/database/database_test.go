package database

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/esp32-rss-display/backend/server/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestNormalizeItemURLsMergesFragmentDuplicates(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	now := time.Now().UTC()
	feed := models.Feed{Name: "V2EX", URL: "https://v2ex.com/index.xml", Enabled: true, CreatedAt: now}
	if err := db.Create(&feed).Error; err != nil {
		t.Fatalf("failed to create feed: %v", err)
	}

	primary := models.Item{FeedID: feed.ID, Title: "topic", URL: "https://www.v2ex.com/t/1", CreatedAt: now}
	duplicate := models.Item{FeedID: feed.ID, Title: "topic", URL: "https://www.v2ex.com/t/1#reply8", CreatedAt: now.Add(time.Minute)}
	if err := db.Create(&primary).Error; err != nil {
		t.Fatalf("failed to create primary item: %v", err)
	}
	if err := db.Create(&duplicate).Error; err != nil {
		t.Fatalf("failed to create duplicate item: %v", err)
	}

	device := models.Device{DeviceID: "esp32-1", CurrentItemID: &duplicate.ID, LastSeen: now, CreatedAt: now}
	show := models.ItemShow{ItemID: duplicate.ID, DeviceID: device.DeviceID, CreatedAt: now}
	read := models.ItemRead{ItemID: duplicate.ID, DeviceID: device.DeviceID, CreatedAt: now}
	rating := models.ItemRating{ItemID: duplicate.ID, DeviceID: device.DeviceID, Rating: 5, CreatedAt: now}
	for _, record := range []any{&device, &show, &read, &rating} {
		if err := db.Create(record).Error; err != nil {
			t.Fatalf("failed to seed record: %v", err)
		}
	}

	if err := normalizeItemURLs(db); err != nil {
		t.Fatalf("normalizeItemURLs failed: %v", err)
	}

	var items []models.Item
	if err := db.Order("id").Find(&items).Error; err != nil {
		t.Fatalf("failed to load items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 merged item, got %d", len(items))
	}
	if items[0].ID != primary.ID {
		t.Fatalf("expected primary item %d to remain, got %d", primary.ID, items[0].ID)
	}
	if items[0].URL != primary.URL {
		t.Fatalf("expected normalized url %q, got %q", primary.URL, items[0].URL)
	}

	var storedDevice models.Device
	if err := db.First(&storedDevice, "device_id = ?", device.DeviceID).Error; err != nil {
		t.Fatalf("failed to load device: %v", err)
	}
	if storedDevice.CurrentItemID == nil || *storedDevice.CurrentItemID != primary.ID {
		t.Fatalf("expected device current item %d, got %+v", primary.ID, storedDevice.CurrentItemID)
	}

	assertRelationItemID(t, db, &models.ItemShow{}, primary.ID)
	assertRelationItemID(t, db, &models.ItemRead{}, primary.ID)
	assertRelationItemID(t, db, &models.ItemRating{}, primary.ID)
}

func TestNormalizeItemURLsUpdatesURLWithoutExistingCanonicalRow(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	now := time.Now().UTC()
	feed := models.Feed{Name: "feed", URL: "https://example.com/feed.xml", Enabled: true, CreatedAt: now}
	if err := db.Create(&feed).Error; err != nil {
		t.Fatalf("failed to create feed: %v", err)
	}

	item := models.Item{FeedID: feed.ID, Title: "topic", URL: "https://example.com/post?id=1#section", CreatedAt: now}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("failed to create item: %v", err)
	}

	if err := normalizeItemURLs(db); err != nil {
		t.Fatalf("normalizeItemURLs failed: %v", err)
	}

	var stored models.Item
	if err := db.First(&stored, item.ID).Error; err != nil {
		t.Fatalf("failed to load item: %v", err)
	}
	if stored.URL != "https://example.com/post?id=1" {
		t.Fatalf("expected normalized url, got %q", stored.URL)
	}
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := filepath.Join(t.TempDir(), "database-test.db")
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	if err := db.AutoMigrate(&models.Device{}, &models.Feed{}, &models.Item{}, &models.ItemShow{}, &models.ItemRead{}, &models.ItemRating{}); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}

	return db
}

func assertRelationItemID(t *testing.T, db *gorm.DB, model any, want uint) {
	t.Helper()

	type row struct {
		ItemID uint
	}

	var rows []row
	if err := db.Model(model).Find(&rows).Error; err != nil {
		t.Fatalf("failed to load relation rows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 relation row, got %d", len(rows))
	}
	if rows[0].ItemID != want {
		t.Fatalf("expected relation item id %d, got %d", want, rows[0].ItemID)
	}
}
