package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Config holds application configuration
type Config struct {
	Port         string
	Environment  string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// loadConfig loads and validates configuration from environment
func loadConfig() (*Config, error) {
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

	return &Config{
		Port:         port,
		Environment:  env,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
	}, nil
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status string `json:"status"`
}

// UploadResponse represents the file upload and scan response
type UploadResponse struct {
	Filename string     `json:"filename"`
	MIME     string     `json:"mime"`
	Scan     *ScanResult `json:"scan"`
	Safe     bool       `json:"safe"`
}

// generateUniqueFilename generates a cryptographically secure random filename
func generateUniqueFilename(originalName string) (string, error) {
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	ext := filepath.Ext(originalName)
	return hex.EncodeToString(randomBytes) + ext, nil
}

// detectMIME detects the MIME type of a file
func detectMIME(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	mimeType := http.DetectContentType(buffer[:n])
	
	// Try to get more specific MIME type using extension
	if mimeType == "application/octet-stream" {
		ext := filepath.Ext(filePath)
		if ext != "" {
			if mimeByExt := mime.TypeByExtension(ext); mimeByExt != "" {
				mimeType = mimeByExt
			}
		}
	}

	return mimeType, nil
}

// setupRouter initializes the Gin router with middleware and routes
func setupRouter(config *AppConfig, scanner *ClamAVScanner) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Info().
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Int("status", c.Writer.Status()).
			Dur("latency", time.Since(start)).
			Msg("request completed")
	})

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, HealthResponse{Status: "ok"})
	})

	// Ping endpoint for testing
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message":   "pong",
			"timestamp": time.Now().UTC(),
		})
	})

	// File upload endpoint
	router.POST("/upload", handleUpload(config, scanner))

	return router
}

// handleUpload handles file upload and ClamAV scanning
func handleUpload(config *AppConfig, scanner *ClamAVScanner) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Create request-scoped logger with trace ID
		traceID := generateTraceID()
		logger := log.With().Str("trace_id", traceID).Logger()
		
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, "trace_id", traceID)

		// Limit request size
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, config.MaxUploadSize)

		// Parse multipart form
		file, header, err := c.Request.FormFile("file")
		if err != nil {
			logger.Warn().Err(err).Msg("failed to parse uploaded file")
			if err.Error() == "http: request body too large" {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{
					"error": fmt.Sprintf("file exceeds maximum size of %d bytes", config.MaxUploadSize),
				})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to parse uploaded file"})
			return
		}
		defer file.Close()

		originalFilename := header.Filename
		logger.Info().Str("filename", originalFilename).Msg("file upload started")

		// Generate unique filename
		uniqueFilename, err := generateUniqueFilename(originalFilename)
		if err != nil {
			logger.Error().Err(err).Msg("failed to generate unique filename")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process file"})
			return
		}

		// Create temp file path
		tempFilePath := filepath.Join(config.TempDir, uniqueFilename)

		// Create temp file with secure permissions
		tempFile, err := os.OpenFile(tempFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			logger.Error().Err(err).Msg("failed to create temp file")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save file"})
			return
		}
		defer func() {
			tempFile.Close()
			// Clean up temp file after response
			if err := os.Remove(tempFilePath); err != nil {
				logger.Warn().Err(err).Str("path", tempFilePath).Msg("failed to remove temp file")
			}
		}()

		// Copy file content
		written, err := io.Copy(tempFile, file)
		if err != nil {
			logger.Error().Err(err).Msg("failed to save file content")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save file"})
			return
		}

		logger.Info().Int64("size", written).Str("path", tempFilePath).Msg("file saved successfully")

		// Detect MIME type
		mimeType, err := detectMIME(tempFilePath)
		if err != nil {
			logger.Warn().Err(err).Msg("failed to detect MIME type")
			// Continue with default MIME type
			mimeType = "application/octet-stream"
		}

		logger.Debug().Str("mime", mimeType).Msg("MIME type detected")

		// Scan file with ClamAV
		scanResult, err := scanner.ScanFile(ctx, tempFilePath)
		if err != nil {
			logger.Error().Err(err).Msg("ClamAV scan failed")
			// Return 503 if ClamAV is unavailable
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "scanning service unavailable",
			})
			return
		}

		response := UploadResponse{
			Filename: originalFilename,
			MIME:     mimeType,
			Scan:     scanResult,
			Safe:     !scanResult.Malicious && scanResult.Clean,
		}

		logger.Info().
			Bool("malicious", scanResult.Malicious).
			Bool("safe", response.Safe).
			Msg("scan completed")

		c.JSON(http.StatusOK, response)
	}
}

// generateTraceID generates a short trace ID for request tracking
func generateTraceID() string {
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(randomBytes)[:16]
}

func main() {
	// Configure zerolog
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	log.Info().Msg("Starting FORGE API server...")

	// Load configuration
	config, err := loadAppConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	log.Info().
		Str("port", config.Port).
		Str("environment", config.Environment).
		Dur("read_timeout", config.ReadTimeout).
		Dur("write_timeout", config.WriteTimeout).
		Int64("max_upload_size", config.MaxUploadSize).
		Str("temp_dir", config.TempDir).
		Str("clamav_host", config.ClamAV.Host).
		Str("clamav_port", config.ClamAV.Port).
		Msg("Configuration loaded")

	// Initialize ClamAV scanner
	scanner := NewClamAVScanner(config.ClamAV)

	// Setup router with config and scanner
	router := setupRouter(config, scanner)

	// Create HTTP server
	srv := &http.Server{
		Addr:         ":" + config.Port,
		Handler:      router,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
	}

	// Channel to listen for errors from goroutine
	errChan := make(chan error, 1)

	// Start server in a goroutine
	go func() {
		log.Info().Str("addr", srv.Addr).Msg("Server starting")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
		close(errChan)
	}()

	// Listen for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Block until signal or error
	select {
	case <-quit:
		log.Info().Msg("Shutdown signal received")
	case err := <-errChan:
		log.Fatal().Err(err).Msg("Server failed")
	}

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Info().Msg("Shutting down server...")
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("Server forced to shutdown")
	}

	log.Info().Msg("Server stopped gracefully")
}
