package config

import (
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type Config struct {
	Port    int
	DBPath  string
	DataDir string

	HAOSURL   string
	HAOSToken string

	PriceImport float64
	PriceExport float64
	Co2PerKwh   float64

	SessionTTL time.Duration
}

func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func Load() (*Config, error) {
	dataDir := os.Getenv("HELIOS_DATA_DIR")
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dataDir = filepath.Join(home, ".local", "share", "helios")
	}

	dbPath := os.Getenv("HELIOS_DB_PATH")
	if dbPath == "" {
		dbPath = filepath.Join(dataDir, "helios.db")
	}

	port := 8080
	if p := os.Getenv("PORT"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			port = parsed
		}
	}

	haosURL := os.Getenv("HAOS_URL")
	if haosURL == "" {
		haosURL = "http://192.168.10.244:8123"
	}

	return &Config{
		Port:      port,
		DBPath:    dbPath,
		DataDir:   dataDir,
		HAOSURL:   haosURL,
		HAOSToken: os.Getenv("HAOS_TOKEN"),

		PriceImport: envFloat("PRICE_IMPORT_EUR", 0.15),
		PriceExport: envFloat("PRICE_EXPORT_EUR", 0.08),
		Co2PerKwh:   envFloat("CO2_KG_PER_KWH", 0.25),

		SessionTTL: 30 * 24 * time.Hour,
	}, nil
}
