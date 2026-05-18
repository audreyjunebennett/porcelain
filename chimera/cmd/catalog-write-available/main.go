// Writes a YAML snapshot of BiFrost's OpenAI-style GET /v1/models response.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/lynn/porcelain/chimera/internal/brokerclient"
	"gopkg.in/yaml.v3"
)

const (
	defaultBaseURL = "http://127.0.0.1:8080"
	defaultOut     = "config/catalog-available.snapshot.yaml"
)

func main() {
	defaultKey := strings.TrimSpace(os.Getenv("CHIMERA_BROKER_API_KEY"))
	baseURL := flag.String("base-url", envOr("BIFROST_BASE_URL", defaultBaseURL), "BiFrost root URL (env BIFROST_BASE_URL)")
	outPath := flag.String("out", defaultOut, "output YAML path")
	timeout := flag.Duration("timeout", 30*time.Second, "HTTP client timeout for GET /v1/models")
	apiKey := flag.String("api-key", defaultKey, "Bearer token (default: env CHIMERA_BROKER_API_KEY)")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout+5*time.Second)
	defer cancel()

	root := strings.TrimSuffix(strings.TrimSpace(*baseURL), "/")
	key := strings.TrimSpace(*apiKey)
	st, body, ok := brokerclient.FetchOpenAIModels(ctx, root, key, *timeout, nil)
	if !ok {
		fmt.Fprintf(os.Stderr, "catalog-write-available: GET %s/v1/models failed (HTTP %d)\n", root, st)
		os.Exit(1)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		fmt.Fprintf(os.Stderr, "catalog-write-available: parse JSON: %v\n", err)
		os.Exit(1)
	}

	data, _ := payload["data"].([]any)
	obj, _ := payload["object"].(string)

	doc := struct {
		FormatVersion int    `yaml:"format_version"`
		FetchedAt     string `yaml:"fetched_at"`
		Source        string `yaml:"source"`
		HTTPStatus    int    `yaml:"http_status"`
		Object        string `yaml:"object,omitempty"`
		Data          []any  `yaml:"data"`
	}{
		FormatVersion: 1,
		FetchedAt:     time.Now().UTC().Format(time.RFC3339),
		Source:        root + "/v1/models",
		HTTPStatus:    st,
		Object:        obj,
		Data:          wrapCatalogData(data),
	}

	var encBuf bytes.Buffer
	enc := yaml.NewEncoder(&encBuf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		fmt.Fprintf(os.Stderr, "catalog-write-available: yaml encode: %v\n", err)
		os.Exit(1)
	}
	if err := enc.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "catalog-write-available: yaml close: %v\n", err)
		os.Exit(1)
	}

	var out bytes.Buffer
	out.WriteString("# catalog-available — BiFrost GET /v1/models (OpenAI-style list).\n")
	out.WriteString("# Re-run: make catalog-available (BiFrost must be reachable).\n")
	out.WriteString("# Env: BIFROST_BASE_URL, CHIMERA_BROKER_API_KEY (optional).\n\n")
	out.Write(encBuf.Bytes())

	if err := os.WriteFile(*outPath, out.Bytes(), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "catalog-write-available: write: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "catalog-write-available: wrote %d models -> %s\n", len(data), *outPath)
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// Keys listed first in each model mapping (remaining keys follow in lexicographic order).
var catalogModelKeyOrder = []string{"id", "name", "description"}

// catalogItem preserves human-friendly field order when encoding model objects to YAML.
type catalogItem map[string]any

func wrapCatalogData(items []any) []any {
	if len(items) == 0 {
		return nil
	}
	out := make([]any, 0, len(items))
	for _, it := range items {
		if m, ok := it.(map[string]any); ok {
			out = append(out, catalogItem(m))
			continue
		}
		out = append(out, it)
	}
	return out
}

func (m catalogItem) MarshalYAML() (interface{}, error) {
	if len(m) == 0 {
		return map[string]any{}, nil
	}
	seen := make(map[string]struct{}, len(m))
	var keys []string
	for _, k := range catalogModelKeyOrder {
		if _, ok := m[k]; !ok {
			continue
		}
		keys = append(keys, k)
		seen[k] = struct{}{}
	}
	var rest []string
	for k := range m {
		if _, done := seen[k]; done {
			continue
		}
		rest = append(rest, k)
	}
	sort.Strings(rest)
	keys = append(keys, rest...)

	var node yaml.Node
	node.Kind = yaml.MappingNode
	for _, k := range keys {
		var kn, vn yaml.Node
		if err := kn.Encode(k); err != nil {
			return nil, err
		}
		if err := vn.Encode(m[k]); err != nil {
			return nil, err
		}
		node.Content = append(node.Content, &kn, &vn)
	}
	return &node, nil
}
