package pipeline

import (
	"path/filepath"
	"testing"

	"github.com/esp32-rss-display/backend/server/config"
	"github.com/esp32-rss-display/backend/server/database"
	"github.com/esp32-rss-display/backend/server/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := gorm.Open(
		sqlite.Open(dbPath),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)},
	)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	if err := db.AutoMigrate(
		&models.Device{},
		&models.Feed{},
		&models.Item{},
		&models.Task{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	database.DB = db
	return db
}

func newPipeline(t *testing.T, pythonPath, scriptPath string) (*Pipeline, string) {
	t.Helper()
	dataDir := filepath.Join(t.TempDir(), "pipeline_data")
	cfg := config.PipelineConfig{
		PythonPath:          pythonPath,
		ScriptPath:          scriptPath,
		DataDir:             dataDir,
		RateLimitMinSeconds: 30,
		RateLimitMaxSeconds: 90,
	}
	p := NewPipeline(cfg)
	if err := p.Init(); err != nil {
		t.Fatalf("failed to init pipeline: %v", err)
	}
	return p, dataDir
}