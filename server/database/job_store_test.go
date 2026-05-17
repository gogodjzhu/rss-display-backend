package database_test

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/esp32-rss-display/backend/server/database"
	"github.com/esp32-rss-display/backend/server/models"
	"github.com/esp32-rss-display/backend/server/pipeline"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Device{}, &models.Feed{}, &models.Item{}, &models.Task{}, &models.Job{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestGORMJobStoreCreateAndLoad(t *testing.T) {
	db := newTestDB(t)
	store := database.NewGORMJobStore(db)

	now := time.Now().Truncate(time.Second)
	job := &pipeline.JobRecord{
		PipelineName: "rss_pipeline",
		Status:       pipeline.JobPending,
		Input:        json.RawMessage(`{"device_id":"dev1"}`),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := store.Create(job); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if job.ID == 0 {
		t.Fatal("Create should set job.ID")
	}

	loaded, err := store.Load(job.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.PipelineName != "rss_pipeline" {
		t.Errorf("PipelineName = %q", loaded.PipelineName)
	}
	if loaded.Status != pipeline.JobPending {
		t.Errorf("Status = %q", loaded.Status)
	}
	if string(loaded.Input) != `{"device_id":"dev1"}` {
		t.Errorf("Input = %q", loaded.Input)
	}
}

func TestGORMJobStoreSave(t *testing.T) {
	db := newTestDB(t)
	store := database.NewGORMJobStore(db)

	now := time.Now().Truncate(time.Second)
	job := &pipeline.JobRecord{
		PipelineName: "rss_pipeline",
		Status:       pipeline.JobPending,
		Input:        json.RawMessage(`{}`),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	_ = store.Create(job)

	completedAt := time.Now().Truncate(time.Second)
	job.Status = pipeline.JobCompleted
	job.Error = ""
	job.CurrentStep = 5
	job.CompletedAt = &completedAt
	job.UpdatedAt = time.Now()

	if err := store.Save(job); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, _ := store.Load(job.ID)
	if loaded.Status != pipeline.JobCompleted {
		t.Errorf("Status = %q, want completed", loaded.Status)
	}
	if loaded.CurrentStep != 5 {
		t.Errorf("CurrentStep = %d, want 5", loaded.CurrentStep)
	}
	if loaded.CompletedAt == nil {
		t.Error("CompletedAt should not be nil")
	}
}

func TestGORMJobStoreLoadPending(t *testing.T) {
	db := newTestDB(t)
	store := database.NewGORMJobStore(db)

	now := time.Now()
	statuses := []pipeline.JobStatus{pipeline.JobPending, pipeline.JobRunning, pipeline.JobCompleted, pipeline.JobFailed}
	for _, s := range statuses {
		_ = store.Create(&pipeline.JobRecord{PipelineName: "p", Status: s, Input: json.RawMessage(`{}`), CreatedAt: now, UpdatedAt: now})
	}

	pending, err := store.LoadPending()
	if err != nil {
		t.Fatalf("LoadPending: %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("LoadPending returned %d records, want 2 (pending+running)", len(pending))
	}
}

func TestGORMJobStoreSetDeviceID(t *testing.T) {
	db := newTestDB(t)
	store := database.NewGORMJobStore(db)

	now := time.Now()
	job := &pipeline.JobRecord{PipelineName: "p", Status: pipeline.JobPending, Input: json.RawMessage(`{}`), CreatedAt: now, UpdatedAt: now}
	_ = store.Create(job)

	if err := store.SetDeviceID(job.ID, "device-abc"); err != nil {
		t.Fatalf("SetDeviceID: %v", err)
	}

	var m models.Job
	if err := db.First(&m, job.ID).Error; err != nil {
		t.Fatalf("fetch job: %v", err)
	}
	if m.DeviceID != "device-abc" {
		t.Errorf("DeviceID = %q, want %q", m.DeviceID, "device-abc")
	}
}

func TestGORMJobStoreLoadMissingReturnsError(t *testing.T) {
	db := newTestDB(t)
	store := database.NewGORMJobStore(db)
	_, err := store.Load(9999)
	if err == nil {
		t.Error("expected error loading non-existent job")
	}
}
