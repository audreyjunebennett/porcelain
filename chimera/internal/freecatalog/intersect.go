package freecatalog

import (
	"sort"
	"strings"
)

func entryMatchesCatalog(e Entry, catalog map[string]struct{}) bool {
	if _, ok := catalog[e.BiFrostID]; ok {
		return true
	}
	src := e.SourceID
	prefix := ""
	switch e.Provider {
	case "groq":
		prefix = "groq/"
	case "gemini":
		prefix = "gemini/"
	default:
		prefix = e.Provider + "/"
	}
	for id := range catalog {
		if !strings.HasPrefix(id, prefix) {
			continue
		}
		tail := strings.TrimPrefix(id, prefix)
		if tail == src {
			return true
		}
		if strings.Contains(tail, src) || strings.Contains(src, tail) {
			if len(src) >= 6 && len(tail) >= 6 {
				return true
			}
		}
	}
	return false
}

// FilterEntriesByCatalog returns entries that match the catalog (non-exact match; see entryMatchesCatalog).
func FilterEntriesByCatalog(entries []Entry, catalogIDs []string) []Entry {
	set := make(map[string]struct{}, len(catalogIDs))
	for _, id := range catalogIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			set[id] = struct{}{}
		}
	}
	var out []Entry
	seen := make(map[string]struct{})
	for _, e := range entries {
		if !entryMatchesCatalog(e, set) {
			continue
		}
		if _, ok := seen[e.BiFrostID]; ok {
			continue
		}
		seen[e.BiFrostID] = struct{}{}
		out = append(out, e)
	}
	return out
}

// AlignEntriesToCatalog replaces BiFrostID with the best-matching id from the live catalog when
// intersect mode is used (prefer longest tail match under the same provider prefix).
func AlignEntriesToCatalog(entries []Entry, catalogIDs []string) []Entry {
	out := make([]Entry, len(entries))
	for i, e := range entries {
		out[i] = e
		out[i].BiFrostID = bestMatchingCatalogID(e, catalogIDs)
	}
	return out
}

func providerPrefix(e Entry) string {
	switch e.Provider {
	case "groq":
		return "groq/"
	case "gemini":
		return "gemini/"
	default:
		if e.Provider != "" {
			return e.Provider + "/"
		}
		return ""
	}
}

func bestMatchingCatalogID(e Entry, catalog []string) string {
	if e.BiFrostID == "" {
		return ""
	}
	for _, id := range catalog {
		if strings.TrimSpace(id) == e.BiFrostID {
			return id
		}
	}
	prefix := providerPrefix(e)
	if prefix == "" {
		return e.BiFrostID
	}
	src := e.SourceID
	var hits []string
	for _, id := range catalog {
		id = strings.TrimSpace(id)
		if !strings.HasPrefix(id, prefix) {
			continue
		}
		tail := strings.TrimPrefix(id, prefix)
		if tail == src {
			return id
		}
		if len(src) >= 6 && len(tail) >= 6 && (strings.Contains(tail, src) || strings.Contains(src, tail)) {
			hits = append(hits, id)
		}
	}
	if len(hits) == 0 {
		return e.BiFrostID
	}
	sort.Slice(hits, func(i, j int) bool {
		if len(hits[i]) != len(hits[j]) {
			return len(hits[i]) > len(hits[j])
		}
		return hits[i] < hits[j]
	})
	return hits[0]
}
