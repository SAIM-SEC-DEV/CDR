package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func getTestConfig() *AppConfig {
	return &AppConfig{
		Port:          "8080",
		Environment:   "test",
		ReadTimeout:   30 * time.Second,
		WriteTimeout:  60 * time.Second,
		MaxUploadSize: 52428800, // 50MB
		TempDir:       "/tmp/forge-test",
		ClamAV: ClamAVConfig{
			Host:    "localhost",
			Port:    "3310",
			Timeout: 30 * time.Second,
		},
	}
}

func TestHealthEndpoint(t *testing.T) {
	config := getTestConfig()
	scanner := NewClamAVScanner(config.ClamAV)
	router := setupRouter(config, scanner)

	req, _ := http.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"status":"ok"}`, w.Body.String())
}

func TestPingEndpoint(t *testing.T) {
	config := getTestConfig()
	scanner := NewClamAVScanner(config.ClamAV)
	router := setupRouter(config, scanner)

	req, _ := http.NewRequest("GET", "/ping", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "pong")
	assert.Contains(t, w.Body.String(), "timestamp")
}

func TestLoadConfig_Defaults(t *testing.T) {
	// Test with no .env file (defaults)
	config, err := loadAppConfig()
	
	assert.NoError(t, err)
	assert.Equal(t, "8080", config.Port)
	assert.Equal(t, "development", config.Environment)
	assert.Equal(t, 30*time.Second, config.ReadTimeout)
	assert.Equal(t, 60*time.Second, config.WriteTimeout)
}

func TestSetupRouter_Routes(t *testing.T) {
	config := getTestConfig()
	scanner := NewClamAVScanner(config.ClamAV)
	router := setupRouter(config, scanner)

	// Test health route exists
	req, _ := http.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test ping route exists
	req, _ = http.NewRequest("GET", "/ping", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test upload route exists (POST)
	req, _ = http.NewRequest("POST", "/upload", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// Should return bad request (no file) not 404
	assert.NotEqual(t, http.StatusNotFound, w.Code)

	// Test non-existent route returns 404
	req, _ = http.NewRequest("GET", "/nonexistent", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
