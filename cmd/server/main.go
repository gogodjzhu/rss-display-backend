package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/esp32-rss-display/backend/server/admin"
	"github.com/esp32-rss-display/backend/server/api"
	"github.com/esp32-rss-display/backend/server/config"
	"github.com/esp32-rss-display/backend/server/database"
	"github.com/esp32-rss-display/backend/server/image"
	"github.com/esp32-rss-display/backend/server/logger"
	"github.com/esp32-rss-display/backend/server/models"
	"github.com/esp32-rss-display/backend/server/pipeline"
	rssworker "github.com/esp32-rss-display/backend/server/rss"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
)

var (
	configPath string
)

var rootCmd = &cobra.Command{
	Run: runServer,
}

func init() {
	rootCmd.Flags().StringVar(&configPath, "config", "config.yaml", "Path to config file")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func runServer(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Log.Dir != "" {
		if err := logger.Init(cfg.Log.Dir); err != nil {
			log.Fatalf("Failed to initialize log directory: %v", err)
		}
	}

	if err := database.Init(&cfg.Database); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	initFeeds(cfg.Feeds)

	pipe := pipeline.NewPipeline(cfg.Pipeline)
	if err := pipe.Init(); err != nil {
		log.Fatalf("Failed to initialize pipeline: %v", err)
	}
	api.InitPipeline(pipe)

	mux := http.NewServeMux()

	mux.Handle("/metrics", promhttp.Handler())
	admin.Mount(mux)
	mux.HandleFunc("GET /v1/device/{device_id}/next", api.GetNextItem)
	mux.HandleFunc("POST /v1/item/{item_id}/rating", api.PostItemRating)
	mux.HandleFunc("PUT /v1/device/{device_id}/preference", api.PutDevicePreference)
	mux.HandleFunc("POST /v1/device/{device_id}/task", api.PostDeviceTask)
	mux.HandleFunc("GET /v1/device/{device_id}/task", api.GetDeviceTask)
	mux.HandleFunc("GET /v1/device/{device_id}/task/{task_id}", api.GetDeviceTaskByID)
	image.Mount(mux, &cfg.RSS)
	mux.HandleFunc("GET /nfc/{device_id}", api.NFCRedirect)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	logger.Get("server").Printf("Starting server on %s", addr)

	worker := rssworker.New(&cfg.RSS)
	worker.Start()

	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	worker.Stop()
	logger.Get("server").Println("Server stopped")
}

func initFeeds(feeds []config.FeedConfig) {
	db := database.GetDB()
	configuredURLs := make(map[string]struct{}, len(feeds))

	logFeed := logger.Get("feed")
	for _, f := range feeds {
		configuredURLs[f.URL] = struct{}{}

		var existing models.Feed
		if err := db.Where("url = ?", f.URL).First(&existing).Error; err != nil {
			db.Create(&models.Feed{
				Name:    f.Name,
				URL:     f.URL,
				Enabled: f.Enabled,
			})
		} else {
			db.Model(&existing).Updates(models.Feed{Name: f.Name, Enabled: f.Enabled})
		}
	}

	var existingFeeds []models.Feed
	if err := db.Find(&existingFeeds).Error; err != nil {
		logFeed.Printf("Failed to list existing feeds: %v", err)
		return
	}

	for _, feed := range existingFeeds {
		if _, ok := configuredURLs[feed.URL]; ok {
			continue
		}
		if !feed.Enabled {
			continue
		}

		if err := db.Model(&feed).Update("enabled", false).Error; err != nil {
			logFeed.Printf("Failed to disable stale feed %q (%s): %v", feed.Name, feed.URL, err)
			continue
		}

		logFeed.Printf("Disabled stale feed %q (%s) not present in config", feed.Name, feed.URL)
	}
}
