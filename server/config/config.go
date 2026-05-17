package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Database DatabaseConfig `yaml:"database"`
	Server   ServerConfig   `yaml:"server"`
	RSS      RSSConfig      `yaml:"rss"`
	Pipeline PipelineConfig `yaml:"pipeline"`
	Log      LogConfig      `yaml:"log"`
	Feeds    []FeedConfig   `yaml:"feeds"`
}

type DatabaseConfig struct {
	Driver string `yaml:"driver"`
	DSN    string `yaml:"dsn"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type RSSConfig struct {
	FetchIntervalMinutes        int `yaml:"fetch_interval_minutes"`
	ImageWidth                  int `yaml:"image_width"`
	ImageHeight                 int `yaml:"image_height"`
	ImageDownloadTimeoutSeconds int `yaml:"image_download_timeout_seconds"`
	FeedFetchTimeoutSeconds     int `yaml:"feed_fetch_timeout_seconds"`
}

type FeedConfig struct {
	Name    string `yaml:"name"`
	URL     string `yaml:"url"`
	Enabled bool   `yaml:"enabled"`
}

type PipelineConfig struct {
	PythonPath string `yaml:"python_path"`
	ScriptPath string `yaml:"script_path"`
	DataDir    string `yaml:"data_dir"`
	// StateDir is where the FileStateManager persists per-job step state.
	// Defaults to <DataDir>/state when empty.
	StateDir            string `yaml:"state_dir"`
	RateLimitMinSeconds int    `yaml:"rate_limit_min_seconds"`
	RateLimitMaxSeconds int    `yaml:"rate_limit_max_seconds"`
}

type LogConfig struct {
	Dir string `yaml:"dir"`
}

var AppConfig *Config

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	AppConfig = &cfg
	return &cfg, nil
}
