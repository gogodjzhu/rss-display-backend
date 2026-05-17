package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/esp32-rss-display/backend/server/models"
	"github.com/esp32-rss-display/backend/server/pipeline"
	"gorm.io/gorm"
)

// FilterL1Step reads items in the configured time range, sends them to the Python
// filter-level-1 script together with device context, and stores the selected IDs
// in state under "filter_l1".
type FilterL1Step struct {
	db     *gorm.DB
	runner *pipeline.PythonRunner
}

// NewFilterL1Step constructs a FilterL1Step.
func NewFilterL1Step(db *gorm.DB, runner *pipeline.PythonRunner) *FilterL1Step {
	return &FilterL1Step{db: db, runner: runner}
}

func (s *FilterL1Step) Name() string { return "filter_l1" }

func (s *FilterL1Step) Config() pipeline.StepConfig {
	return pipeline.StepConfig{
		Timeout:     10 * time.Minute,
		RetryPolicy: pipeline.RetryPolicy{MaxAttempts: 2, BaseDelay: 5 * time.Second, MaxDelay: 30 * time.Second},
	}
}

func (s *FilterL1Step) Run(ctx context.Context, state pipeline.StateAccessor) error {
	input, err := pipeline.GetState[RSSJobInput](state, "job_input")
	if err != nil {
		return err
	}

	device, err := getDevice(s.db, input.DeviceID)
	if err != nil {
		return fmt.Errorf("filter_l1: get device: %w", err)
	}

	items, err := getItemsInRange(s.db, input.TimeRangeStart, input.TimeRangeEnd)
	if err != nil {
		return fmt.Errorf("filter_l1: get items: %w", err)
	}

	type itemEntry struct {
		ID    uint   `json:"id"`
		Title string `json:"title"`
		URL   string `json:"url"`
	}
	entries := make([]itemEntry, len(items))
	for i, item := range items {
		entries[i] = itemEntry{ID: item.ID, Title: item.Title, URL: item.URL}
	}

	pyInput := map[string]any{
		"device": map[string]string{
			"role":       device.Role,
			"preference": device.Preference,
		},
		"items": entries,
	}

	inPath, outPath, cleanup := tempIOPaths("filter_l1")
	defer cleanup()

	if err := s.runner.WriteJSON(inPath, pyInput); err != nil {
		return fmt.Errorf("filter_l1: write input: %w", err)
	}
	if err := s.runner.RunCtx(ctx, "filter_l1", inPath, outPath); err != nil {
		return fmt.Errorf("filter_l1: python: %w", err)
	}

	var pyOutput struct {
		Level1IDs []uint `json:"level1_ids"`
	}
	if err := s.runner.ReadJSON(outPath, &pyOutput); err != nil {
		return fmt.Errorf("filter_l1: read output: %w", err)
	}

	return pipeline.SetState(state, s.Name(), FilterL1Output{Level1IDs: pyOutput.Level1IDs})
}

// getDevice fetches a Device by deviceID, creating a minimal one if absent.
func getDevice(db *gorm.DB, deviceID string) (*models.Device, error) {
	var device models.Device
	err := db.Where("device_id = ?", deviceID).First(&device).Error
	if err != nil {
		if isNotFound(err) {
			device = models.Device{DeviceID: deviceID, CreatedAt: time.Now(), LastSeen: time.Now()}
			if createErr := db.Create(&device).Error; createErr != nil {
				return nil, fmt.Errorf("create device: %w", createErr)
			}
			return &device, nil
		}
		return nil, fmt.Errorf("get device: %w", err)
	}
	return &device, nil
}

// getItemsInRange returns items whose published_at or created_at falls in [start, end].
func getItemsInRange(db *gorm.DB, start, end time.Time) ([]models.Item, error) {
	var items []models.Item
	err := db.Where(
		"(published_at BETWEEN ? AND ?) OR (published_at IS NULL AND created_at BETWEEN ? AND ?)",
		start, end, start, end,
	).Find(&items).Error
	if err != nil {
		return nil, fmt.Errorf("get items in range: %w", err)
	}
	return items, nil
}

func isNotFound(err error) bool {
	return err != nil && err.Error() == "record not found"
}

// tempIOPaths returns temp file paths for Python I/O and a cleanup function.
func tempIOPaths(prefix string) (inPath, outPath string, cleanup func()) {
	ts := time.Now().UnixNano()
	inPath = filepath.Join(os.TempDir(), fmt.Sprintf("%s_%d_in.json", prefix, ts))
	outPath = filepath.Join(os.TempDir(), fmt.Sprintf("%s_%d_out.json", prefix, ts))
	cleanup = func() {
		os.Remove(inPath)
		os.Remove(outPath)
	}
	return
}
