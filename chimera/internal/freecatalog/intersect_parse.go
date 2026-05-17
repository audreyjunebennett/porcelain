package freecatalog

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseCatalogIntersect extracts BiFrost-style model ids from JSON or YAML shaped like
// OpenAI GET /v1/models: a top-level "data" array of objects with string field "id".
func ParseCatalogIntersect(raw []byte) ([]string, error) {
	raw = trimUTF8BOM(raw)
	var doc catalogIntersectDoc
	jErr := json.Unmarshal(raw, &doc)
	if jErr == nil {
		return doc.nonEmptyIDs(), nil
	}
	doc = catalogIntersectDoc{}
	if yErr := yaml.Unmarshal(raw, &doc); yErr != nil {
		return nil, fmt.Errorf("parse intersect catalog (JSON or YAML with data[].id): json: %v; yaml: %w", jErr, yErr)
	}
	return doc.nonEmptyIDs(), nil
}

type catalogIntersectDoc struct {
	Data []catalogIntersectRow `json:"data" yaml:"data"`
}

type catalogIntersectRow struct {
	ID string `json:"id" yaml:"id"`
}

func (d catalogIntersectDoc) nonEmptyIDs() []string {
	var out []string
	for _, row := range d.Data {
		id := strings.TrimSpace(row.ID)
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

func trimUTF8BOM(b []byte) []byte {
	s := strings.TrimPrefix(string(b), "\ufeff")
	return []byte(s)
}
