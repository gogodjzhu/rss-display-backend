package pipeline

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/esp32-rss-display/backend/server/config"
	"github.com/esp32-rss-display/backend/server/database"
	"github.com/esp32-rss-display/backend/server/logger"
	"github.com/esp32-rss-display/backend/server/models"
	"gorm.io/gorm"
)

var pipelineLog = logger.Get("pipeline")

var (
	runningMu     sync.Mutex
	runningTaskID *uint
)

var ErrTaskRunning = fmt.Errorf("a pipeline task is already running")

type Pipeline struct {
	runner *PythonRunner
	cfg    config.PipelineConfig
}

func NewPipeline(cfg config.PipelineConfig) *Pipeline {
	runner := NewPythonRunner(PythonConfig{
		PythonPath: cfg.PythonPath,
		ScriptPath: cfg.ScriptPath,
		DataDir:    cfg.DataDir,
	})
	return &Pipeline{
		runner: runner,
		cfg:    cfg,
	}
}

func (p *Pipeline) Init() error {
	return p.runner.EnsureDataDir()
}

type StartResult struct {
	Task *models.Task
}

func (p *Pipeline) StartPipeline(deviceID string, timeStart, timeEnd time.Time) (*models.Task, error) {
	runningMu.Lock()
	defer runningMu.Unlock()

	if runningTaskID != nil {
		return nil, ErrTaskRunning
	}

	db := database.GetDB()

	device, err := ensureDevice(db, deviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure device: %w", err)
	}

	task := &models.Task{
		DeviceID:       device.DeviceID,
		Status:         "pending",
		TimeRangeStart: &timeStart,
		TimeRangeEnd:   &timeEnd,
	}

	if err := db.Create(task).Error; err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	runningTaskID = &task.ID
	taskIDCopy := task.ID

	go func() {
		defer func() {
			runningMu.Lock()
			if runningTaskID != nil && *runningTaskID == taskIDCopy {
				runningTaskID = nil
			}
			runningMu.Unlock()
		}()

		p.runPipeline(taskIDCopy)
	}()

	return task, nil
}

func (p *Pipeline) IsRunning() bool {
	runningMu.Lock()
	defer runningMu.Unlock()
	return runningTaskID != nil
}

type stage struct {
	name string
	mode string
}

var stages = []stage{
	{name: "filtering_l1", mode: "filter_l1"},
	{name: "crawling", mode: "crawl"},
	{name: "summarizing", mode: "summarize"},
	{name: "filtering_l2", mode: "filter_l2"},
	{name: "composing", mode: "compose"},
}

func (p *Pipeline) runPipeline(taskID uint) {
	db := database.GetDB()

	var task models.Task
	if err := db.First(&task, taskID).Error; err != nil {
		pipelineLog.Printf("task %d not found: %v", taskID, err)
		return
	}

	for _, s := range stages {
		pipelineLog.Printf("task %d: starting stage %s", taskID, s.name)

		if err := db.Model(&task).Update("status", s.name).Error; err != nil {
			p.failTask(db, &task, fmt.Sprintf("failed to update status to %s: %v", s.name, err))
			return
		}

		inputData, err := p.buildInput(s.mode, &task, db)
		if err != nil {
			p.failTask(db, &task, fmt.Sprintf("failed to build input for %s: %v", s.name, err))
			return
		}

		inputPath := p.runner.InputPath(taskID, s.mode)
		outputPath := p.runner.OutputPath(taskID, s.mode)

		if err := p.runner.WriteJSON(inputPath, inputData); err != nil {
			p.failTask(db, &task, fmt.Sprintf("failed to write input for %s: %v", s.name, err))
			return
		}

		if err := p.runner.Run(s.mode, inputPath, outputPath); err != nil {
			p.failTask(db, &task, fmt.Sprintf("python %s failed: %v", s.mode, err))
			return
		}

		if err := p.applyResult(s.mode, &task, outputPath, db); err != nil {
			p.failTask(db, &task, fmt.Sprintf("failed to apply result for %s: %v", s.name, err))
			return
		}

		pipelineLog.Printf("task %d: completed stage %s", taskID, s.name)

		if err := db.First(&task, taskID).Error; err != nil {
			pipelineLog.Printf("task %d: failed to reload task: %v", taskID, err)
			return
		}
	}

	now := time.Now()
	db.Model(&task).Updates(map[string]any{
		"status":       "completed",
		"completed_at": &now,
	})
	pipelineLog.Printf("task %d: completed", taskID)
}

func (p *Pipeline) failTask(db *gorm.DB, task *models.Task, errMsg string) {
	pipelineLog.Printf("task %d: %s", task.ID, errMsg)
	db.Model(task).Updates(map[string]any{
		"status": "failed",
		"error":  errMsg,
	})
}

func (p *Pipeline) buildInput(mode string, task *models.Task, db *gorm.DB) (any, error) {
	switch mode {
	case "filter_l1":
		return p.buildFilterL1Input(task, db)
	case "crawl":
		return p.buildCrawlInput(task, db)
	case "summarize":
		return p.buildSummarizeInput(task, db)
	case "filter_l2":
		return p.buildFilterL2Input(task, db)
	case "compose":
		return p.buildComposeInput(task, db)
	default:
		return nil, fmt.Errorf("unknown mode: %s", mode)
	}
}

func (p *Pipeline) buildFilterL1Input(task *models.Task, db *gorm.DB) (any, error) {
	device, err := getDevice(db, task.DeviceID)
	if err != nil {
		return nil, err
	}

	items, err := getItemsInRange(db, *task.TimeRangeStart, *task.TimeRangeEnd)
	if err != nil {
		return nil, err
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

	return map[string]any{
		"device": map[string]string{
			"role":       device.Role,
			"preference": device.Preference,
		},
		"items": entries,
	}, nil
}

func (p *Pipeline) buildCrawlInput(task *models.Task, db *gorm.DB) (any, error) {
	ids, err := parseIDsFromJSON(task.Level1IDs)
	if err != nil {
		return nil, fmt.Errorf("failed to parse level1_ids: %w", err)
	}

	var items []models.Item
	if err := db.Where("id IN ?", ids).Find(&items).Error; err != nil {
		return nil, err
	}

	type itemEntry struct {
		ID  uint   `json:"id"`
		URL string `json:"url"`
	}

	entries := make([]itemEntry, len(items))
	for i, item := range items {
		entries[i] = itemEntry{ID: item.ID, URL: item.URL}
	}

	return map[string]any{
		"rate_limit_min_seconds": p.cfg.RateLimitMinSeconds,
		"rate_limit_max_seconds": p.cfg.RateLimitMaxSeconds,
		"items":                  entries,
	}, nil
}

func (p *Pipeline) buildSummarizeInput(task *models.Task, db *gorm.DB) (any, error) {
	ids, err := parseIDsFromJSON(task.Level1IDs)
	if err != nil {
		return nil, fmt.Errorf("failed to parse level1_ids: %w", err)
	}

	var items []models.Item
	if err := db.Where("id IN ? AND content != ''", ids).Find(&items).Error; err != nil {
		return nil, err
	}

	type itemEntry struct {
		ID      uint   `json:"id"`
		Title   string `json:"title"`
		Content string `json:"content"`
	}

	entries := make([]itemEntry, len(items))
	for i, item := range items {
		entries[i] = itemEntry{ID: item.ID, Title: item.Title, Content: item.Content}
	}

	return map[string]any{
		"items": entries,
	}, nil
}

func (p *Pipeline) buildFilterL2Input(task *models.Task, db *gorm.DB) (any, error) {
	device, err := getDevice(db, task.DeviceID)
	if err != nil {
		return nil, err
	}

	ids, err := parseIDsFromJSON(task.Level1IDs)
	if err != nil {
		return nil, fmt.Errorf("failed to parse level1_ids: %w", err)
	}

	var items []models.Item
	if err := db.Where("id IN ? AND abstract != ''", ids).Find(&items).Error; err != nil {
		return nil, err
	}

	type itemEntry struct {
		ID       uint   `json:"id"`
		Title    string `json:"title"`
		Abstract string `json:"abstract"`
	}

	entries := make([]itemEntry, len(items))
	for i, item := range items {
		entries[i] = itemEntry{ID: item.ID, Title: item.Title, Abstract: item.Abstract}
	}

	return map[string]any{
		"device": map[string]string{
			"role":       device.Role,
			"preference": device.Preference,
		},
		"items": entries,
	}, nil
}

func (p *Pipeline) buildComposeInput(task *models.Task, db *gorm.DB) (any, error) {
	device, err := getDevice(db, task.DeviceID)
	if err != nil {
		return nil, err
	}

	ids, err := parseIDsFromJSON(task.Level2IDs)
	if err != nil {
		return nil, fmt.Errorf("failed to parse level2_ids: %w", err)
	}

	var items []models.Item
	if err := db.Where("id IN ?", ids).Find(&items).Error; err != nil {
		return nil, err
	}

	type itemEntry struct {
		ID       uint   `json:"id"`
		Title    string `json:"title"`
		Abstract string `json:"abstract"`
		URL      string `json:"url"`
	}

	entries := make([]itemEntry, len(items))
	for i, item := range items {
		entries[i] = itemEntry{ID: item.ID, Title: item.Title, Abstract: item.Abstract, URL: item.URL}
	}

	return map[string]any{
		"device": map[string]string{
			"role":       device.Role,
			"preference": device.Preference,
		},
		"items": entries,
	}, nil
}

func (p *Pipeline) applyResult(mode string, task *models.Task, outputPath string, db *gorm.DB) error {
	switch mode {
	case "filter_l1":
		return p.applyFilterL1Result(task, outputPath, db)
	case "crawl":
		return p.applyCrawlResult(task, outputPath, db)
	case "summarize":
		return p.applySummarizeResult(task, outputPath, db)
	case "filter_l2":
		return p.applyFilterL2Result(task, outputPath, db)
	case "compose":
		return p.applyComposeResult(task, outputPath, db)
	default:
		return fmt.Errorf("unknown mode: %s", mode)
	}
}

func (p *Pipeline) applyFilterL1Result(task *models.Task, outputPath string, db *gorm.DB) error {
	var result struct {
		Level1IDs []uint `json:"level1_ids"`
	}
	if err := p.runner.ReadJSON(outputPath, &result); err != nil {
		return err
	}

	idsJSON, err := json.Marshal(result.Level1IDs)
	if err != nil {
		return err
	}

	return db.Model(task).Update("level1_ids", string(idsJSON)).Error
}

func (p *Pipeline) applyCrawlResult(task *models.Task, outputPath string, db *gorm.DB) error {
	var result struct {
		Results []struct {
			ID      uint   `json:"id"`
			Content string `json:"content"`
			Success bool   `json:"success"`
			Error   string `json:"error"`
		} `json:"results"`
	}
	if err := p.runner.ReadJSON(outputPath, &result); err != nil {
		return err
	}

	for _, r := range result.Results {
		if r.Success && r.Content != "" {
			if err := db.Model(&models.Item{}).Where("id = ?", r.ID).Update("content", r.Content).Error; err != nil {
				pipelineLog.Printf("failed to update content for item %d: %v", r.ID, err)
			}
		} else {
			pipelineLog.Printf("crawl failed for item %d: %s", r.ID, r.Error)
		}
	}

	return nil
}

func (p *Pipeline) applySummarizeResult(task *models.Task, outputPath string, db *gorm.DB) error {
	var result struct {
		Results []struct {
			ID       uint   `json:"id"`
			Abstract string `json:"abstract"`
		} `json:"results"`
	}
	if err := p.runner.ReadJSON(outputPath, &result); err != nil {
		return err
	}

	for _, r := range result.Results {
		if r.Abstract != "" {
			if err := db.Model(&models.Item{}).Where("id = ?", r.ID).Update("abstract", r.Abstract).Error; err != nil {
				pipelineLog.Printf("failed to update abstract for item %d: %v", r.ID, err)
			}
		}
	}

	return nil
}

func (p *Pipeline) applyFilterL2Result(task *models.Task, outputPath string, db *gorm.DB) error {
	var result struct {
		Level2IDs []uint `json:"level2_ids"`
	}
	if err := p.runner.ReadJSON(outputPath, &result); err != nil {
		return err
	}

	idsJSON, err := json.Marshal(result.Level2IDs)
	if err != nil {
		return err
	}

	return db.Model(task).Update("level2_ids", string(idsJSON)).Error
}

func (p *Pipeline) applyComposeResult(task *models.Task, outputPath string, db *gorm.DB) error {
	var result struct {
		Report string `json:"report"`
	}
	if err := p.runner.ReadJSON(outputPath, &result); err != nil {
		return err
	}

	return db.Model(task).Update("report", result.Report).Error
}

func ensureDevice(db *gorm.DB, deviceID string) (*models.Device, error) {
	var device models.Device
	if err := db.Where("device_id = ?", deviceID).First(&device).Error; err != nil {
		device = models.Device{
			DeviceID:  deviceID,
			CreatedAt: time.Now(),
			LastSeen:  time.Now(),
		}
		if err := db.Create(&device).Error; err != nil {
			return nil, err
		}
	}
	return &device, nil
}

func getDevice(db *gorm.DB, deviceID string) (*models.Device, error) {
	var device models.Device
	if err := db.Where("device_id = ?", deviceID).First(&device).Error; err != nil {
		return nil, err
	}
	return &device, nil
}

func getItemsInRange(db *gorm.DB, start, end time.Time) ([]models.Item, error) {
	var items []models.Item
	err := db.Where(
		db.Where("published_at BETWEEN ? AND ?", start, end).
			Or("published_at IS NULL AND created_at BETWEEN ? AND ?", start, end),
	).Order("id ASC").
		Find(&items).Error
	return items, err
}

func parseIDsFromJSON(jsonStr string) ([]uint, error) {
	if jsonStr == "" {
		return nil, nil
	}
	var ids []uint
	if err := json.Unmarshal([]byte(jsonStr), &ids); err != nil {
		return nil, err
	}
	return ids, nil
}