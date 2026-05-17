package steps_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/esp32-rss-display/backend/server/domain/devices"
	"github.com/esp32-rss-display/backend/server/domain/items"
	"github.com/esp32-rss-display/backend/server/models"
	"github.com/esp32-rss-display/backend/server/pipeline"
	"github.com/esp32-rss-display/backend/server/pipeline/steps"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Device{}, &models.Feed{}, &models.Item{}, &models.Job{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func newState(t *testing.T) pipeline.StateAccessor {
	t.Helper()
	fsm := pipeline.NewFileStateManager(t.TempDir())
	acc, err := fsm.Open(1)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	return acc
}

func seedDB(t *testing.T, db *gorm.DB) (models.Device, models.Feed, []models.Item) {
	t.Helper()
	device := models.Device{
		DeviceID:   "dev-001",
		Role:       "engineer",
		Preference: "Go, distributed systems",
		CreatedAt:  time.Now(),
		LastSeen:   time.Now(),
	}
	db.Create(&device)

	feed := models.Feed{Name: "Tech", URL: "https://example.com/rss", Enabled: true}
	db.Create(&feed)

	pub := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	itemList := []models.Item{
		{FeedID: feed.ID, Title: "Go 1.24", URL: "https://example.com/go124", PublishedAt: &pub},
		{FeedID: feed.ID, Title: "Kubernetes Tips", URL: "https://example.com/k8s", PublishedAt: &pub},
		{FeedID: feed.ID, Title: "Celebrity Gossip", URL: "https://example.com/gossip", PublishedAt: &pub},
	}
	for i := range itemList {
		db.Create(&itemList[i])
	}
	return device, feed, itemList
}

// stub implementations for domain interfaces

type stubDeviceGetter struct {
	device *models.Device
	err    error
}

func (s *stubDeviceGetter) GetOrCreate(ctx context.Context, deviceID string) (*models.Device, error) {
	return s.device, s.err
}

type stubItemRanger struct {
	items []models.Item
	err   error
}

func (s *stubItemRanger) FindByTimeRange(ctx context.Context, start, end time.Time) ([]models.Item, error) {
	return s.items, s.err
}

type stubItemFinder struct {
	items []models.Item
	err   error
}

func (s *stubItemFinder) FindByIDs(ctx context.Context, ids []uint) ([]models.Item, error) {
	return s.items, s.err
}

type stubItemUpdater struct{}

func (s *stubItemUpdater) UpdateContent(ctx context.Context, id uint, content string) error { return nil }
func (s *stubItemUpdater) UpdateAbstract(ctx context.Context, id uint, abstract string) error { return nil }

type stubJobReporter struct{}

func (s *stubJobReporter) UpdateReport(ctx context.Context, jobID uint, report string, level2IDs []uint) error {
	return nil
}

// ── FilterL1Step ──────────────────────────────────────────────────────────────

func TestFilterL1StepName(t *testing.T) {
	s := steps.NewFilterL1Step(&stubDeviceGetter{}, &stubItemRanger{}, &pipeline.PythonRunner{})
	if s.Name() != "filter_l1" {
		t.Errorf("Name() = %q, want %q", s.Name(), "filter_l1")
	}
}

func TestFilterL1StepConfigHasTimeout(t *testing.T) {
	s := steps.NewFilterL1Step(&stubDeviceGetter{}, &stubItemRanger{}, &pipeline.PythonRunner{})
	cfg := s.Config()
	if cfg.Timeout <= 0 {
		t.Error("FilterL1Step config timeout should be positive")
	}
	if cfg.RetryPolicy.MaxAttempts < 1 {
		t.Error("FilterL1Step retry policy MaxAttempts should be at least 1")
	}
}

func TestFilterL1StepRunFailsWithoutJobInput(t *testing.T) {
	s := steps.NewFilterL1Step(&stubDeviceGetter{}, &stubItemRanger{}, &pipeline.PythonRunner{})
	state := newState(t)

	err := s.Run(context.Background(), state)
	if err == nil {
		t.Error("Run without job_input in state should return error")
	}
}

// ── CrawlStep ────────────────────────────────────────────────────────────────

func TestCrawlStepName(t *testing.T) {
	s := steps.NewCrawlStep(&stubItemFinder{}, &stubItemUpdater{}, &pipeline.PythonRunner{}, 30, 90)
	if s.Name() != "crawl" {
		t.Errorf("Name() = %q, want %q", s.Name(), "crawl")
	}
}

func TestCrawlStepSkipsEmptyLevel1IDs(t *testing.T) {
	s := steps.NewCrawlStep(&stubItemFinder{}, &stubItemUpdater{}, &pipeline.PythonRunner{}, 30, 90)
	state := newState(t)

	input := steps.RSSJobInput{DeviceID: "dev-001", TimeRangeStart: time.Now(), TimeRangeEnd: time.Now()}
	pipeline.SetState(state, "job_input", input)
	pipeline.SetState(state, "filter_l1", steps.FilterL1Output{Level1IDs: nil})

	if err := s.Run(context.Background(), state); err != nil {
		t.Fatalf("Run with empty Level1IDs should succeed, got: %v", err)
	}

	result, err := pipeline.GetState[steps.CrawlOutput](state, "crawl")
	if err != nil {
		t.Fatalf("GetState crawl: %v", err)
	}
	if len(result.CrawledIDs) != 0 {
		t.Errorf("expected empty CrawledIDs for empty input, got %v", result.CrawledIDs)
	}
}

func TestCrawlStepFailsWithoutFilterL1State(t *testing.T) {
	s := steps.NewCrawlStep(&stubItemFinder{}, &stubItemUpdater{}, &pipeline.PythonRunner{}, 30, 90)
	state := newState(t)

	err := s.Run(context.Background(), state)
	if err == nil {
		t.Error("Run without filter_l1 in state should return error")
	}
}

// ── SummarizeStep ────────────────────────────────────────────────────────────

func TestSummarizeStepName(t *testing.T) {
	s := steps.NewSummarizeStep(&stubItemFinder{}, &stubItemUpdater{}, &pipeline.PythonRunner{})
	if s.Name() != "summarize" {
		t.Errorf("Name() = %q, want %q", s.Name(), "summarize")
	}
}

func TestSummarizeStepSkipsEmptyCrawledIDs(t *testing.T) {
	s := steps.NewSummarizeStep(&stubItemFinder{}, &stubItemUpdater{}, &pipeline.PythonRunner{})
	state := newState(t)

	pipeline.SetState(state, "crawl", steps.CrawlOutput{CrawledIDs: nil})

	if err := s.Run(context.Background(), state); err != nil {
		t.Fatalf("Run with empty CrawledIDs should succeed, got: %v", err)
	}

	result, err := pipeline.GetState[steps.SummarizeOutput](state, "summarize")
	if err != nil {
		t.Fatalf("GetState summarize: %v", err)
	}
	if len(result.SummarizedIDs) != 0 {
		t.Errorf("expected empty SummarizedIDs, got %v", result.SummarizedIDs)
	}
}

// ── FilterL2Step ────────────────────────────────────────────────────────────

func TestFilterL2StepName(t *testing.T) {
	s := steps.NewFilterL2Step(&stubDeviceGetter{}, &stubItemFinder{}, &pipeline.PythonRunner{})
	if s.Name() != "filter_l2" {
		t.Errorf("Name() = %q, want %q", s.Name(), "filter_l2")
	}
}

func TestFilterL2StepSkipsEmptySummarizedIDs(t *testing.T) {
	s := steps.NewFilterL2Step(&stubDeviceGetter{}, &stubItemFinder{}, &pipeline.PythonRunner{})
	state := newState(t)

	input := steps.RSSJobInput{DeviceID: "dev-001", TimeRangeStart: time.Now(), TimeRangeEnd: time.Now()}
	pipeline.SetState(state, "job_input", input)
	pipeline.SetState(state, "summarize", steps.SummarizeOutput{SummarizedIDs: nil})

	if err := s.Run(context.Background(), state); err != nil {
		t.Fatalf("Run with empty SummarizedIDs should succeed, got: %v", err)
	}

	result, err := pipeline.GetState[steps.FilterL2Output](state, "filter_l2")
	if err != nil {
		t.Fatalf("GetState filter_l2: %v", err)
	}
	if len(result.Level2IDs) != 0 {
		t.Errorf("expected empty Level2IDs, got %v", result.Level2IDs)
	}
}

// ── ComposeStep ────────────────────────────────────────────────────────────

func TestComposeStepName(t *testing.T) {
	s := steps.NewComposeStep(&stubDeviceGetter{}, &stubItemFinder{}, &stubJobReporter{}, &pipeline.PythonRunner{})
	if s.Name() != "compose" {
		t.Errorf("Name() = %q, want %q", s.Name(), "compose")
	}
}

func TestComposeStepFailsWithoutFilterL2State(t *testing.T) {
	s := steps.NewComposeStep(&stubDeviceGetter{}, &stubItemFinder{}, &stubJobReporter{}, &pipeline.PythonRunner{})
	state := newState(t)

	err := s.Run(context.Background(), state)
	if err == nil {
		t.Error("Run without filter_l2 in state should return error")
	}
}

// ── BuildRSSPipeline ──────────────────────────────────────────────────────────

func TestBuildRSSPipelineHasFiveSteps(t *testing.T) {
	db := newDB(t)
	deviceGetter := devices.NewService(devices.NewGORMRepository(db))
	itemFinder := items.NewService(items.NewGORMRepository(db))
	itemRanger := items.NewService(items.NewGORMRepository(db))
	itemUpdater := items.NewService(items.NewGORMRepository(db))
	jobReporter := &stubJobReporter{}
	runner := &pipeline.PythonRunner{PythonPath: "python3", ScriptPath: "py/pipeline.py"}
	p := steps.BuildRSSPipeline(deviceGetter, itemFinder, itemRanger, itemUpdater, jobReporter, runner, 30, 90)

	if p.Name() != steps.PipelineName {
		t.Errorf("pipeline name = %q, want %q", p.Name(), steps.PipelineName)
	}
}

func TestRSSJobInputRoundTrip(t *testing.T) {
	state := newState(t)
	now := time.Now().Truncate(time.Second).UTC()
	input := steps.RSSJobInput{
		DeviceID:       "dev-abc",
		TimeRangeStart: now,
		TimeRangeEnd:   now.Add(24 * time.Hour),
	}

	b, _ := json.Marshal(input)
	pipeline.SetState(state, "job_input", input)

	got, err := pipeline.GetState[steps.RSSJobInput](state, "job_input")
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if got.DeviceID != input.DeviceID {
		t.Errorf("DeviceID = %q, want %q", got.DeviceID, input.DeviceID)
	}
	_ = b
}