package envfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// UpsertResult describes whether an environment variable was written.
type UpsertResult struct {
	Written bool
	Reason  string // "created", "updated", "already_set", "unchanged"
}

// UpsertIfAbsent sets key=value in a dotenv file when key is not already assigned
// to a non-empty value. Commented lines and blank values do not count as set.
func UpsertIfAbsent(path, key, value string) (UpsertResult, error) {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" {
		return UpsertResult{}, fmt.Errorf("env key is required")
	}
	if value == "" {
		return UpsertResult{}, fmt.Errorf("env value is required")
	}

	prefix := key + "="
	var lines []string
	if raw, err := os.ReadFile(path); err == nil {
		lines = strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	} else if !os.IsNotExist(err) {
		return UpsertResult{}, err
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		body := trimmed
		if strings.HasPrefix(strings.ToLower(body), "export ") {
			body = strings.TrimSpace(body[7:])
		}
		if !strings.HasPrefix(body, prefix) {
			continue
		}
		existing := strings.TrimSpace(strings.TrimPrefix(body, prefix))
		if existing != "" {
			return UpsertResult{Written: false, Reason: "already_set"}, nil
		}
	}

	needsNL := len(lines) > 0 && lines[len(lines)-1] != ""
	out := strings.Join(lines, "\n")
	if needsNL && out != "" {
		out += "\n"
	}
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	out += prefix + value + "\n"

	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return UpsertResult{}, err
		}
	}
	reason := "created"
	if len(lines) > 0 {
		reason = "updated"
	}
	if err := os.WriteFile(path, []byte(out), 0o600); err != nil {
		return UpsertResult{}, err
	}
	return UpsertResult{Written: true, Reason: reason}, nil
}
