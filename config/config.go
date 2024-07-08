package config

import (
	"log"
	"os"
	"strconv"
)

type Config struct {
	LogFilePath string
	MetricsPort int
	LogLevel    string
}

func LoadConfig() Config {
	config := Config{
		LogFilePath: "/opt/adguardhome/work/data/querylog.json",
		MetricsPort: 8000,
		LogLevel:    "INFO",
	}

	if envLogFilePath := os.Getenv("LOG_FILE_PATH"); envLogFilePath != "" {
		config.LogFilePath = envLogFilePath
	}

	if envMetricsPort := os.Getenv("METRICS_PORT"); envMetricsPort != "" {
		if port, err := strconv.Atoi(envMetricsPort); err == nil {
			config.MetricsPort = port
		} else {
			log.Printf("Invalid METRICS_PORT value: %s. Using default: %d", envMetricsPort, config.MetricsPort)
		}
	}

	if envLogLevel := os.Getenv("LOG_LEVEL"); envLogLevel != "" {
		config.LogLevel = envLogLevel
	}

	return config
}
