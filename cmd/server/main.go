package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/esp32-rss-display/backend/server/admin"
	"github.com/esp32-rss-display/backend/server/api"
	"github.com/esp32-rss-display/backend/server/config"
	"github.com/esp32-rss-display/backend/server/database"
	"github.com/esp32-rss-display/backend/server/domain/feeds"
	"github.com/esp32-rss-display/backend/server/image"
	"github.com/esp32-rss-display/backend/server/logger"
	"github.com/esp32-rss-display/backend/server/pipeline"
	"github.com/esp32-rss-display/backend/server/pipeline/steps"
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

	feedSvc := feeds.NewService(feeds.NewGORMRepository())
	db := database.GetDB()
	if err := feedSvc.InitFeeds(context.Background(), db, cfg.Feeds); err != nil {
		log.Fatalf("Failed to initialise feeds: %v", err)
	}

	// Build the pipeline runner.
	stateDir := cfg.Pipeline.StateDir
	if stateDir == "" {
		stateDir = filepath.Join(cfg.Pipeline.DataDir, "state")
	}
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		log.Fatalf("Failed to create pipeline state dir: %v", err)
	}
	pythonRunner := &pipeline.PythonRunner{
		PythonPath: cfg.Pipeline.PythonPath,
		ScriptPath: cfg.Pipeline.ScriptPath,
		DataDir:    cfg.Pipeline.DataDir,
	}
	jobStore := database.NewGORMJobStore(db)
	stateManager := pipeline.NewFileStateManager(stateDir)
	runner := pipeline.NewRunner(jobStore, stateManager, log.Default())
	rssPipeline := steps.BuildRSSPipeline(db, pythonRunner, cfg.Pipeline.RateLimitMinSeconds, cfg.Pipeline.RateLimitMaxSeconds)
	if err := runner.Register(rssPipeline); err != nil {
		log.Fatalf("Failed to register RSS pipeline: %v", err)
	}
	if err := runner.Recover(context.Background()); err != nil {
		log.Fatalf("Failed to recover pending pipeline jobs: %v", err)
	}
	api.InitRunner(runner)

	mux := http.NewServeMux()

	mux.Handle("/metrics", promhttp.Handler())
	admin.Mount(mux)
	mux.HandleFunc("GET /v1/device/{device_id}/next", api.GetNextItem)
	mux.HandleFunc("POST /v1/item/{item_id}/rating", api.PostItemRating)
	mux.HandleFunc("PUT /v1/device/{device_id}/preference", api.PutDevicePreference)
	mux.HandleFunc("POST /v1/device/{device_id}/job", api.PostDeviceJob)
	mux.HandleFunc("GET /v1/device/{device_id}/job", api.GetDeviceJob)
	mux.HandleFunc("GET /v1/device/{device_id}/job/{job_id}", api.GetDeviceJobByID)
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
