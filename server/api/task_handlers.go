package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/esp32-rss-display/backend/server/database"
	"github.com/esp32-rss-display/backend/server/models"
	"github.com/esp32-rss-display/backend/server/pipeline"
	"gorm.io/gorm"
)

var defaultPipeline *pipeline.Pipeline

func InitPipeline(p *pipeline.Pipeline) {
	defaultPipeline = p
}

type PreferenceRequest struct {
	Role       string `json:"role"`
	Preference string `json:"preference"`
}

type PreferenceResponse struct {
	DeviceID   string `json:"device_id"`
	Role       string `json:"role"`
	Preference string `json:"preference"`
}

type CreateTaskRequest struct {
	TimeRangeStart string `json:"time_range_start"`
	TimeRangeEnd   string `json:"time_range_end"`
}

type TaskResponse struct {
	ID             uint    `json:"id"`
	DeviceID       string  `json:"device_id"`
	Status         string  `json:"status"`
	TimeRangeStart *string `json:"time_range_start,omitempty"`
	TimeRangeEnd   *string `json:"time_range_end,omitempty"`
	Level1IDs      string  `json:"level1_ids,omitempty"`
	Level2IDs      string  `json:"level2_ids,omitempty"`
	Report         string  `json:"report,omitempty"`
	Error          string  `json:"error,omitempty"`
	CreatedAt      string  `json:"created_at"`
	CompletedAt    *string `json:"completed_at,omitempty"`
}

func PutDevicePreference(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	var req PreferenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	db := database.GetDB()

	var device models.Device
	err := db.Where("device_id = ?", deviceID).First(&device).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		device = models.Device{
			DeviceID:    deviceID,
			Role:        req.Role,
			Preference:  req.Preference,
			LastSeen:    time.Now(),
			CreatedAt:   time.Now(),
		}
		if err := db.Create(&device).Error; err != nil {
			http.Error(w, "failed to create device", http.StatusInternalServerError)
			return
		}
	} else if err != nil {
		http.Error(w, "failed to load device", http.StatusInternalServerError)
		return
	} else {
		updates := map[string]any{}
		if req.Role != "" {
			updates["role"] = req.Role
		}
		if req.Preference != "" {
			updates["preference"] = req.Preference
		}
		if len(updates) > 0 {
			if err := db.Model(&device).Updates(updates).Error; err != nil {
				http.Error(w, "failed to update preference", http.StatusInternalServerError)
				return
			}
		}
	}

	if err := db.Where("device_id = ?", deviceID).First(&device).Error; err != nil {
		http.Error(w, "failed to reload device", http.StatusInternalServerError)
		return
	}

	resp := PreferenceResponse{
		DeviceID:   device.DeviceID,
		Role:       device.Role,
		Preference: device.Preference,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func PostDeviceTask(w http.ResponseWriter, r *http.Request) {
	if defaultPipeline == nil {
		http.Error(w, "pipeline not initialized", http.StatusInternalServerError)
		return
	}

	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	var req CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.TimeRangeStart == "" || req.TimeRangeEnd == "" {
		http.Error(w, "time_range_start and time_range_end are required", http.StatusBadRequest)
		return
	}

	timeStart, err := time.Parse(time.RFC3339, req.TimeRangeStart)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid time_range_start format: %v", err), http.StatusBadRequest)
		return
	}

	timeEnd, err := time.Parse(time.RFC3339, req.TimeRangeEnd)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid time_range_end format: %v", err), http.StatusBadRequest)
		return
	}

	task, err := defaultPipeline.StartPipeline(deviceID, timeStart, timeEnd)
	if err != nil {
		if errors.Is(err, pipeline.ErrTaskRunning) {
			http.Error(w, "a pipeline task is already running", http.StatusConflict)
			return
		}
		apiLog.Printf("failed to start pipeline: %v", err)
		http.Error(w, "failed to start pipeline", http.StatusInternalServerError)
		return
	}

	resp := taskToResponse(task)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func GetDeviceTask(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	db := database.GetDB()

	var task models.Task
	if err := db.Where("device_id = ?", deviceID).Order("created_at DESC, id DESC").First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "no tasks found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load task", http.StatusInternalServerError)
		return
	}

	resp := taskToResponse(&task)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func GetDeviceTaskByID(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	taskIDStr := r.PathValue("task_id")
	if deviceID == "" {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	var taskID uint
	if _, err := fmt.Sscanf(taskIDStr, "%d", &taskID); err != nil {
		http.Error(w, "invalid task_id", http.StatusBadRequest)
		return
	}

	db := database.GetDB()

	var task models.Task
	if err := db.Where("id = ? AND device_id = ?", taskID, deviceID).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "task not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load task", http.StatusInternalServerError)
		return
	}

	resp := taskToResponse(&task)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func taskToResponse(task *models.Task) TaskResponse {
	resp := TaskResponse{
		ID:        task.ID,
		DeviceID:  task.DeviceID,
		Status:    task.Status,
		Level1IDs: task.Level1IDs,
		Level2IDs: task.Level2IDs,
		Report:    task.Report,
		Error:     task.Error,
		CreatedAt: task.CreatedAt.Format(time.RFC3339),
	}

	if task.TimeRangeStart != nil {
		s := task.TimeRangeStart.Format(time.RFC3339)
		resp.TimeRangeStart = &s
	}
	if task.TimeRangeEnd != nil {
		s := task.TimeRangeEnd.Format(time.RFC3339)
		resp.TimeRangeEnd = &s
	}
	if task.CompletedAt != nil {
		s := task.CompletedAt.Format(time.RFC3339)
		resp.CompletedAt = &s
	}

	return resp
}