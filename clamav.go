package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/rs/zerolog/log"
)

// ScanResult represents the result of a ClamAV scan
type ScanResult struct {
	Malicious  bool     `json:"malicious"`
	Signatures []string `json:"signatures"`
	Clean      bool     `json:"clean"`
	Error      string   `json:"error,omitempty"`
}

// ClamAVScanner handles ClamAV scanning operations
type ClamAVScanner struct {
	config ClamAVConfig
}

// NewClamAVScanner creates a new ClamAV scanner instance
func NewClamAVScanner(config ClamAVConfig) *ClamAVScanner {
	return &ClamAVScanner{config: config}
}

// IsAvailable checks if ClamAV daemon is reachable
func (s *ClamAVScanner) IsAvailable(ctx context.Context) bool {
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var conn net.Conn
	var err error

	if s.config.UseUnixSock {
		conn, err = dialCtx.Value("dialer").(func(string, string) (net.Conn, error))(
			"unix", s.config.UnixSocketPath,
		)
		if err != nil {
			conn, err = net.DialTimeout("unix", s.config.UnixSocketPath, 5*time.Second)
		}
	} else {
		addr := fmt.Sprintf("%s:%s", s.config.Host, s.config.Port)
		conn, err = net.DialTimeout("tcp", addr, 5*time.Second)
	}

	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// ScanFile scans a file using ClamAV daemon
func (s *ClamAVScanner) ScanFile(ctx context.Context, filePath string) (*ScanResult, error) {
	// Create context with timeout
	scanCtx, cancel := context.WithTimeout(ctx, s.config.Timeout)
	defer cancel()

	result := &ScanResult{
		Malicious:  false,
		Signatures: make([]string, 0),
		Clean:      true,
	}

	// Check if ClamAV is available first
	if !s.IsAvailable(scanCtx) {
		result.Clean = false
		result.Error = "ClamAV daemon is not available"
		return result, fmt.Errorf("clamav daemon is not available")
	}

	// Connect to ClamAV daemon
	var conn net.Conn
	var err error

	if s.config.UseUnixSock {
		conn, err = net.DialTimeout("unix", s.config.UnixSocketPath, s.config.Timeout)
	} else {
		addr := fmt.Sprintf("%s:%s", s.config.Host, s.config.Port)
		conn, err = net.DialTimeout("tcp", addr, s.config.Timeout)
	}

	if err != nil {
		result.Clean = false
		result.Error = fmt.Sprintf("failed to connect to ClamAV: %v", err)
		return result, fmt.Errorf("failed to connect to ClamAV: %w", err)
	}
	defer conn.Close()

	// Set read/write deadlines
	conn.SetDeadline(time.Now().Add(s.config.Timeout))

	// Send INSTREAM command for streaming scan
	// Format: nnnn where nnnn is the length of the chunk in hex
	// For simplicity, we'll use the SCAN command with file path
	// In production, you'd want to stream the file content
	
	// Using zINSTREAM for streaming the file content
	file, err := openFileForScan(filePath)
	if err != nil {
		result.Clean = false
		result.Error = fmt.Sprintf("failed to open file: %v", err)
		return result, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Send zINSTREAM command (null-terminated)
	_, err = conn.Write([]byte("zINSTREAM\x00"))
	if err != nil {
		result.Clean = false
		result.Error = fmt.Sprintf("failed to send scan command: %v", err)
		return result, fmt.Errorf("failed to send scan command: %w", err)
	}

	// Stream file content in chunks
	buffer := make([]byte, 4096)
	for {
		n, err := file.Read(buffer)
		if n > 0 {
			// Send chunk size as 8 hex digits followed by data
			chunkSize := fmt.Sprintf("%08x", n)
			_, writeErr := conn.Write([]byte(chunkSize))
			if writeErr != nil {
				result.Clean = false
				result.Error = fmt.Sprintf("failed to write chunk size: %v", writeErr)
				return result, writeErr
			}
			_, writeErr = conn.Write(buffer[:n])
			if writeErr != nil {
				result.Clean = false
				result.Error = fmt.Sprintf("failed to write chunk: %v", writeErr)
				return result, writeErr
			}
		}
		if err != nil {
			break
		}
	}

	// Send end of stream marker (8 zeros)
	_, err = conn.Write([]byte("00000000"))
	if err != nil {
		result.Clean = false
		result.Error = fmt.Sprintf("failed to send end marker: %v", err)
		return result, fmt.Errorf("failed to send end marker: %w", err)
	}

	// Read response
	response := make([]byte, 4096)
	n, err := conn.Read(response)
	if err != nil && err.Error() != "EOF" {
		log.Warn().Err(err).Msg("error reading ClamAV response")
	}

	if n > 0 {
		responseStr := string(response[:n])
		log.Debug().Str("response", responseStr).Msg("ClamAV scan response")

		// Parse response format: stream: OK or stream: VIRUS_NAME FOUND
		if containsMalicious(responseStr) {
			result.Malicious = true
			result.Clean = false
			// Extract signature name
			signature := extractSignature(responseStr)
			if signature != "" {
				result.Signatures = append(result.Signatures, signature)
			}
		}
	}

	return result, nil
}

// openFileForScan opens a file for scanning
func openFileForScan(filePath string) (*os.File, error) {
	return os.Open(filePath)
}

// containsMalicious checks if the response indicates malware
func containsMalicious(response string) bool {
	// ClamAV returns "stream: OK" for clean files
	// and "stream: VIRUS_NAME FOUND" for infected files
	return len(response) > 0 && 
		   (contains(response, "FOUND") || 
		    contains(response, "END-OF-STREAM"))
}

// extractSignature extracts the virus signature name from response
func extractSignature(response string) string {
	// Response format: "stream: Win32.EICAR_Test FOUND"
	// We want to extract "Win32.EICAR_Test"
	if idx := indexOf(response, ":"); idx != -1 {
		if idx2 := indexOf(response, " FOUND"); idx2 != -1 && idx2 > idx {
			return trimSpace(response[idx+1 : idx2])
		}
	}
	return ""
}

// Helper functions to avoid additional imports
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
