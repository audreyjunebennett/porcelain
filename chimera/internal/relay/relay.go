package relay

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"time"
)

// Config holds relay server settings
type Config struct {
	Token       string
	Port        int
	LocusPort   int
	LocusHost   string
}

// GenerateToken creates a random 32-character hex token
func GenerateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// StartRelayServer spawns the Python relay server as a subprocess
func StartRelayServer(token string, port int, log *slog.Logger) error {
	cmd := exec.Command(
		"python",
		"locus/relay_server.py",
	)

	// Set environment variables for the relay
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("LOCUS_RELAY_TOKEN=%s", token),
		fmt.Sprintf("LOCUS_RELAY_PORT=%d", port),
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start in background
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start relay server: %w", err)
	}

	// Log startup
	log.Info("relay server started", "port", port, "token_len", len(token))

	// Wait briefly to ensure it's listening
	time.Sleep(500 * time.Millisecond)
	return nil
}

// SpawnLocusServer spawns the Locus Python API server as a subprocess
func SpawnLocusServer(log *slog.Logger) error {
	cmd := exec.Command("python", "locus_api.py")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("spawn locus: %w", err)
	}

	log.Info("locus server spawned", "pid", cmd.Process.Pid)
	return nil
}

// GetLocalIP returns the machine's local LAN IP
func GetLocalIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

// GetPublicIP attempts to fetch the public IP (falls back to local IP for home setup)
func GetPublicIP(log *slog.Logger) string {
	// For local home setup, we use the local IP
	// In production, you could call an external service to get public IP
	ip, err := GetLocalIP()
	if err != nil {
		log.Warn("get local ip failed, using localhost", "err", err)
		return "localhost"
	}
	return ip
}

// CheckRelayHealth tests if the relay server is responding
func CheckRelayHealth(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()
	return true
}
