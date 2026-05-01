package config

import "testing"

func TestLoadConfigUsesDefaults(t *testing.T) {
	t.Setenv("LOG_FILE_PATH", "")
	t.Setenv("METRICS_PORT", "")
	t.Setenv("LOG_LEVEL", "")

	cfg := LoadConfig()

	if cfg.LogFilePath != "/opt/adguardhome/work/data/querylog.json" {
		t.Fatalf("expected default log file path, got %q", cfg.LogFilePath)
	}
	if cfg.MetricsPort != 8000 {
		t.Fatalf("expected default metrics port 8000, got %d", cfg.MetricsPort)
	}
	if cfg.LogLevel != "INFO" {
		t.Fatalf("expected default log level INFO, got %q", cfg.LogLevel)
	}
}

func TestLoadConfigUsesEnvironmentOverrides(t *testing.T) {
	t.Setenv("LOG_FILE_PATH", "/tmp/querylog.json")
	t.Setenv("METRICS_PORT", "9100")
	t.Setenv("LOG_LEVEL", "DEBUG")

	cfg := LoadConfig()

	if cfg.LogFilePath != "/tmp/querylog.json" {
		t.Fatalf("expected env log file path, got %q", cfg.LogFilePath)
	}
	if cfg.MetricsPort != 9100 {
		t.Fatalf("expected env metrics port 9100, got %d", cfg.MetricsPort)
	}
	if cfg.LogLevel != "DEBUG" {
		t.Fatalf("expected env log level DEBUG, got %q", cfg.LogLevel)
	}
}

func TestLoadConfigFallsBackToDefaultPortWhenEnvPortIsInvalid(t *testing.T) {
	t.Setenv("LOG_FILE_PATH", "")
	t.Setenv("METRICS_PORT", "not-a-port")
	t.Setenv("LOG_LEVEL", "")

	cfg := LoadConfig()

	if cfg.MetricsPort != 8000 {
		t.Fatalf("expected invalid metrics port to fall back to 8000, got %d", cfg.MetricsPort)
	}
}

func TestLoadConfigFallsBackToDefaultPortWhenEnvPortIsOutOfRange(t *testing.T) {
	tests := []string{"0", "-1", "65536"}

	for _, envPort := range tests {
		t.Run(envPort, func(t *testing.T) {
			t.Setenv("LOG_FILE_PATH", "")
			t.Setenv("METRICS_PORT", envPort)
			t.Setenv("LOG_LEVEL", "")

			cfg := LoadConfig()

			if cfg.MetricsPort != 8000 {
				t.Fatalf("expected out-of-range metrics port %q to fall back to 8000, got %d", envPort, cfg.MetricsPort)
			}
		})
	}
}
