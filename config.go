package main

import (
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
)

// ClamAVConfig holds ClamAV connection configuration
type ClamAVConfig struct {
	Host       string
	Port       string
	UseUnixSock bool
	UnixSocketPath string
	Timeout    time.Duration
}

// AppConfig holds all application configuration
type AppConfig struct {
	Port              string
	Environment       string
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	MaxUploadSize     int64
	TempDir           string
	ClamAV            ClamAVConfig
}

// loadAppConfig loads and validates all configuration from environment
func loadAppConfig() (*AppConfig, error) {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Warn().Msg("No .env file found, using environment variables")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	env := os.Getenv("ENVIRONMENT")
	if env == "" {
		env = "development"
	}

	readTimeoutStr := os.Getenv("READ_TIMEOUT")
	if readTimeoutStr == "" {
		readTimeoutStr = "30s"
	}
	readTimeout, err := time.ParseDuration(readTimeoutStr)
	if err != nil {
		return nil, fmt.Errorf("invalid READ_TIMEOUT: %w", err)
	}

	writeTimeoutStr := os.Getenv("WRITE_TIMEOUT")
	if writeTimeoutStr == "" {
		writeTimeoutStr = "60s"
	}
	writeTimeout, err := time.ParseDuration(writeTimeoutStr)
	if err != nil {
		return nil, fmt.Errorf("invalid WRITE_TIMEOUT: %w", err)
	}

	// Max upload size (default 50MB)
	maxUploadSizeStr := os.Getenv("MAX_UPLOAD_SIZE")
	if maxUploadSizeStr == "" {
		maxUploadSizeStr = "52428800" // 50MB in bytes
	}
	var maxUploadSize int64
	_, err = fmt.Sscanf(maxUploadSizeStr, "%d", &maxUploadSize)
	if err != nil {
		return nil, fmt.Errorf("invalid MAX_UPLOAD_SIZE: %w", err)
	}

	// Temp directory
	tempDir := os.Getenv("TEMP_DIR")
	if tempDir == "" {
		tempDir = "/tmp/forge"
	}

	// Create temp dir if it doesn't exist
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	// ClamAV configuration
	clamHost := os.Getenv("CLAMAV_HOST")
	if clamHost == "" {
		clamHost = "localhost"
	}

	clamPort := os.Getenv("CLAMAV_PORT")
	if clamPort == "" {
		clamPort = "3310"
	}

	useUnixSock := os.Getenv("CLAMAV_USE_UNIX_SOCK") == "true"
	unixSocketPath := os.Getenv("CLAMAV_UNIX_SOCKET_PATH")
	if unixSocketPath == "" {
		unixSocketPath = "/var/run/clamav/clamd.ctl"
	}

	clamTimeoutStr := os.Getenv("CLAMAV_TIMEOUT")
	if clamTimeoutStr == "" {
		clamTimeoutStr = "30s"
	}
	clamTimeout, err := time.ParseDuration(clamTimeoutStr)
	if err != nil {
		return nil, fmt.Errorf("invalid CLAMAV_TIMEOUT: %w", err)
	}

	return &AppConfig{
		Port:         port,
		Environment:  env,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		MaxUploadSize: maxUploadSize,
		TempDir:      tempDir,
		ClamAV: ClamAVConfig{
			Host:           clamHost,
			Port:           clamPort,
			UseUnixSock:    useUnixSock,
			UnixSocketPath: unixSocketPath,
			Timeout:        clamTimeout,
		},
	}, nil
}
