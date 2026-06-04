package indexer

import (
	"path/filepath"
	"strings"
)

// rootIdentityKey identifies a watch root for diffing (path + ingest scope).
func rootIdentityKey(r Root) string {
	return strings.Join([]string{
		filepath.Clean(r.AbsPath),
		strings.TrimSpace(r.Scope.WorkspaceID),
		strings.TrimSpace(r.Scope.ProjectID),
		strings.TrimSpace(r.Scope.FlavorID),
	}, "\x1f")
}

// DiffRoots compares previous and next materialised roots. Added and removed
// slices reference entries from next and prev respectively.
func DiffRoots(prev, next []Root) (added, removed []Root) {
	prevBy := make(map[string]Root, len(prev))
	for _, r := range prev {
		prevBy[rootIdentityKey(r)] = r
	}
	nextBy := make(map[string]Root, len(next))
	for _, r := range next {
		nextBy[rootIdentityKey(r)] = r
	}
	for k, r := range nextBy {
		if _, ok := prevBy[k]; !ok {
			added = append(added, r)
		}
	}
	for k, r := range prevBy {
		if _, ok := nextBy[k]; !ok {
			removed = append(removed, r)
		}
	}
	return added, removed
}
