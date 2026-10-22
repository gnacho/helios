package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	Port int
	DBPath string
	DataDir string
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
		// parse in main; keep simple here
		port = 8080
	}

	return &Config{
		Port: port,
		DBPath: dbPath,
		DataDir: dataDir,
	}, nil
}
