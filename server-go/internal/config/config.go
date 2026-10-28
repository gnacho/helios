package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port    int
	DBPath  string
	DataDir string

	HAOSURL           string
	HAOSToken         string
	HAOSAllowInsecure bool

	PriceImport float64
	PriceExport float64
	Co2PerKwh   float64

	SessionTTL     time.Duration
	TrustedProxies []string

	VAPIDPublicKey  string
	VAPIDPrivateKey string
	VAPIDSubject    string
}

func envList(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	var values []string
	for _, item := range strings.Split(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			values = append(values, item)
		}
	}
	return values
}

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
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
		dataDir = os.Getenv("DATA_DIR") // compat con el .env del servidor Node
	}
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

		HAOSAllowInsecure: envBool("HAOS_ALLOW_INSECURE"),

		PriceImport: envFloat("PRICE_IMPORT_EUR", 0.15),
		PriceExport: envFloat("PRICE_EXPORT_EUR", 0.08),
		Co2PerKwh:   envFloat("CO2_KG_PER_KWH", 0.25),

		SessionTTL:     30 * 24 * time.Hour,
		TrustedProxies: envList("TRUSTED_PROXIES"),

		VAPIDPublicKey:  os.Getenv("VAPID_PUBLIC_KEY"),
		VAPIDPrivateKey: os.Getenv("VAPID_PRIVATE_KEY"),
		VAPIDSubject:    os.Getenv("VAPID_SUBJECT"),
	}, nil
}
