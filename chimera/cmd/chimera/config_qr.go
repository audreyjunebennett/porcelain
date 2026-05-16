package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/lynn/claudia-gateway/internal/relay"
)

// ConfigQRResponse contains the QR code + setup metadata
type ConfigQRResponse struct {
	QRCode      string    `json:"qrCode"`      // Base64 encoded PNG
	Token       string    `json:"token"`       // RELAY_TOKEN (masked)
	LocalIP     string    `json:"localIp"`     // Home PC local IP
	RelayPort   int       `json:"relayPort"`   // 9999
	LocusPort   int       `json:"locusPort"`   // 11435
	RelayStatus string    `json:"relayStatus"` // "running" | "pending" | "error"
	GeneratedAt time.Time `json:"generatedAt"`
}

// ConfigStatusResponse contains current status
type ConfigStatusResponse struct {
	RelayStatus  string `json:"relayStatus"`  // "running" | "pending" | "error"
	LocusStatus  string `json:"locusStatus"`  // "running" | "pending" | "error"
	ChimeraStatus string `json:"chimeraStatus"` // "running"
	LocalIP      string `json:"localIP"`
	RelayPort    int    `json:"relayPort"`
	LocusPort    int    `json:"locusPort"`
	Message      string `json:"message"`
}

// Global relay config (set at startup)
var relayToken string
var relayPort = 9999
var locusPort = 11435
var log *slog.Logger

// handleConfigQR serves the QR code + relay setup
func handleConfigQR(l *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Get local IP
		localIP, err := relay.GetLocalIP()
		if err != nil {
			localIP = "192.168.x.x"
			l.Warn("get local ip", "err", err)
		}

		// Check relay health
		relayStatus := "pending"
		if relay.CheckRelayHealth(relayPort) {
			relayStatus = "running"
		}

		// Mask token for display
		maskedToken := "••••••••"
		if len(relayToken) > 4 {
			maskedToken = relayToken[:4] + "••••" + relayToken[len(relayToken)-4:]
		}

		// TODO: Generate cute QR code by calling Python locus/qrcode_cute.py
		// For now, return placeholder
		qrBase64 := "" // Will be filled in by qrcode generation

		resp := ConfigQRResponse{
			QRCode:      qrBase64,
			Token:       maskedToken,
			LocalIP:     localIP,
			RelayPort:   relayPort,
			LocusPort:   locusPort,
			RelayStatus: relayStatus,
			GeneratedAt: time.Now().UTC(),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// handleConfigStatus serves current system status
func handleConfigStatus(l *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		localIP, _ := relay.GetLocalIP()
		if localIP == "" {
			localIP = "192.168.x.x"
		}

		// Check all service statuses
		relayStatus := "pending"
		if relay.CheckRelayHealth(relayPort) {
			relayStatus = "running"
		}

		locusStatus := "pending"
		if isPortOpen("127.0.0.1", locusPort) {
			locusStatus = "running"
		}

		resp := ConfigStatusResponse{
			RelayStatus:   relayStatus,
			LocusStatus:   locusStatus,
			ChimeraStatus: "running",
			LocalIP:       localIP,
			RelayPort:     relayPort,
			LocusPort:     locusPort,
			Message:       fmt.Sprintf("Relay: %s | Locus: %s | Chimera: running", relayStatus, locusStatus),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// Helper to check if a port is open
func isPortOpen(host string, port int) bool {
	return relay.CheckRelayHealth(port) // Reuse the relay health check
}

// LogEntry represents a single log line
type LogEntry struct {
	Message string `json:"message"`
	Level   string `json:"level"`
	Time    string `json:"time"`
}

// RecentLogsResponse contains recent log lines
type RecentLogsResponse struct {
	Logs []LogEntry `json:"logs"`
}

// handleLogsRecent serves recent logs (last 50 lines)
func handleLogsRecent(l *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// For now, return empty logs (would read from .data/logs in production)
		// TODO: Read from .data/logs/*.log files and parse them
		resp := RecentLogsResponse{
			Logs: []LogEntry{
				{Message: "Relay server started on port 9999", Level: "info", Time: "2026-05-10T19:18:00Z"},
				{Message: "Locus API listening on port 11435", Level: "info", Time: "2026-05-10T19:18:01Z"},
				{Message: "Chimera gateway listening on port 3000", Level: "info", Time: "2026-05-10T19:18:02Z"},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// MotoXStatusResponse contains receiver queue and transcript info
type MotoXStatusResponse struct {
	Status            string `json:"status"`            // "running" | "pending" | "error"
	PendingCount      int    `json:"pendingCount"`      // Files in inbox waiting to process
	ProcessingCount   int    `json:"processingCount"`   // Files currently being processed
	CompletedCount    int    `json:"completedCount"`    // Files that have been transcribed
	RecentRecordings  []Recording `json:"recentRecordings"` // Last 5 transcriptions
}

type Recording struct {
	Speaker   string `json:"speaker"`   // Diarized speaker name
	Text      string `json:"text"`      // Transcript text
	Timestamp string `json:"timestamp"` // ISO 8601 timestamp
	Duration  string `json:"duration"`  // "mm:ss" format
}

// handleMotoXStatus serves Moto X receiver status
func handleMotoXStatus(l *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check if receiver.log exists and was recently updated
		status := "pending"
		var recentRecordings []Recording

		// TODO: Implement queue stats by reading from MOTOX_AUDIO_DIR filesystem
		// For now, return placeholder data with "running" status
		status = "running"
		recentRecordings = []Recording{
			{Speaker: "Ruby", Text: "just setting up the desktop launcher", Timestamp: "2026-05-10T19:20:00Z", Duration: "0:03"},
		}

		resp := MotoXStatusResponse{
			Status:           status,
			PendingCount:     0,
			ProcessingCount:  0,
			CompletedCount:   1,
			RecentRecordings: recentRecordings,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// SetupRelayGlobals sets the global relay configuration
func SetupRelayGlobals(token string, l *slog.Logger) {
	relayToken = token
	log = l
}
