package config

import (
	"cxar/rodiger.io/internal/types"
	"encoding/json"
	"os"
	"time"
)

type Config struct {
	ListenAddr     string         `json:"listen_addr"`
	UpdateInterval types.Duration `json:"update_interval"`
	DocID          string         `json:"doc_id"`
	GoogleCredPath string         `json:"google_cred_path"`
}

func Load() (*Config, error) {
	// First try environment variables
	cfg := &Config{
		ListenAddr:     getEnv("LISTEN_ADDR", ":8080"),
		UpdateInterval: types.Duration(getDurationEnv("UPDATE_INTERVAL", 1*time.Hour)),
		DocID:          os.Getenv("GOOGLE_DOC_ID"),
		GoogleCredPath: getEnv("GOOGLE_CRED_PATH", "credentials.json"),
	}

	// If config file exists, overlay those settings
	if _, err := os.Stat("config.json"); err == nil {
		file, err := os.ReadFile("config.json")
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(file, cfg); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	if value, ok := os.LookupEnv(key); ok {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return fallback
}
