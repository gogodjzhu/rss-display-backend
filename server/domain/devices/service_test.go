package devices_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/esp32-rss-display/backend/server/domain/devices"
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
	if err := db.AutoMigrate(&models.Device{}, &models.Job{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestGetOrCreateCreatesNewDevice(t *testing.T) {
	db := newTestDB(t)
	svc := devices.NewService(devices.NewGORMRepository(db))

	device, err := svc.GetOrCreate(context.Background(), "dev-1")
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if device.DeviceID != "dev-1" {
		t.Fatalf("expected device_id %q, got %q", "dev-1", device.DeviceID)
	}
}

func TestGetOrCreateReturnsExisting(t *testing.T) {
	db := newTestDB(t)
	svc := devices.NewService(devices.NewGORMRepository(db))

	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	existing := models.Device{DeviceID: "dev-2", CreatedAt: now, LastSeen: now}
	db.Create(&existing)

	device, err := svc.GetOrCreate(context.Background(), "dev-2")
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if device.DeviceID != "dev-2" {
		t.Fatalf("expected device_id %q, got %q", "dev-2", device.DeviceID)
	}
	var count int64
	db.Model(&models.Device{}).Where("device_id = ?", "dev-2").Count(&count)
	if count != 1 {
		t.Fatalf("expected exactly 1 device row, count=%d", count)
	}
}

func TestUpdatePreferenceSetsFields(t *testing.T) {
	db := newTestDB(t)
	svc := devices.NewService(devices.NewGORMRepository(db))

	existing := models.Device{DeviceID: "dev-3", CreatedAt: time.Now(), LastSeen: time.Now()}
	db.Create(&existing)

	updated, err := svc.UpdatePreference(context.Background(), "dev-3", "news", "compact")
	if err != nil {
		t.Fatalf("UpdatePreference: %v", err)
	}
	if updated.Role != "news" {
		t.Fatalf("expected role %q, got %q", "news", updated.Role)
	}
	if updated.Preference != "compact" {
		t.Fatalf("expected preference %q, got %q", "compact", updated.Preference)
	}
}

func TestUpdatePreferenceCreatesDevice(t *testing.T) {
	db := newTestDB(t)
	svc := devices.NewService(devices.NewGORMRepository(db))

	device, err := svc.UpdatePreference(context.Background(), "brand-new", "tech", "full")
	if err != nil {
		t.Fatalf("UpdatePreference: %v", err)
	}
	if device.DeviceID != "brand-new" {
		t.Fatalf("expected device_id %q, got %q", "brand-new", device.DeviceID)
	}
}