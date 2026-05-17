package steps

import (
	"context"
	"fmt"
	"time"

	"github.com/esp32-rss-display/backend/server/pipeline"
)

// FilterL1Step reads items in the configured time range, sends them to the Python
// filter-level-1 script together with device context, and stores the selected IDs
// in state under "filter_l1".
type FilterL1Step struct {
	devices DeviceGetter
	items   ItemRanger
	runner  *pipeline.PythonRunner
}

// NewFilterL1Step constructs a FilterL1Step.
func NewFilterL1Step(devices DeviceGetter, items ItemRanger, runner *pipeline.PythonRunner) *FilterL1Step {
	return &FilterL1Step{devices: devices, items: items, runner: runner}
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

	device, err := s.devices.GetOrCreate(ctx, input.DeviceID)
	if err != nil {
		return fmt.Errorf("filter_l1: get device: %w", err)
	}

	itemList, err := s.items.FindByTimeRange(ctx, input.TimeRangeStart, input.TimeRangeEnd)
	if err != nil {
		return fmt.Errorf("filter_l1: get items: %w", err)
	}

	type itemEntry struct {
		ID    uint   `json:"id"`
		Title string `json:"title"`
		URL   string `json:"url"`
	}
	entries := make([]itemEntry, len(itemList))
	for i, item := range itemList {
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