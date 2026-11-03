package contentresource

import (
	"path"
	"strings"
)

// CanonicalResourcePath builds the JRXML-facing path for an Image Resource.
// folderSlash is the folder path without a trailing filename ("" or "/david").
// leaf is the file name (e.g. "family.png").
// Result examples: "/family.png", "/david/family.png".
func CanonicalResourcePath(folderSlash, leaf string) string {
	leaf = ResourcePathLeaf(leaf)
	if leaf == "" {
		return ""
	}
	folder := strings.Trim(strings.ReplaceAll(folderSlash, `\`, `/`), "/")
	if folder == "" {
		return "/" + leaf
	}
	return "/" + folder + "/" + leaf
}

// ResourcePathLeaf returns a safe basename for path identity (forward slashes).
func ResourcePathLeaf(name string) string {
	name = strings.TrimSpace(strings.ReplaceAll(name, `\`, `/`))
	if name == "" {
		return ""
	}
	return path.Base(name)
}

// NormalizeResourceLookupPath converts a JRXML image expression location into the
// canonical form used for lookup ("/david/family.png").
func NormalizeResourceLookupPath(location string) string {
	location = strings.TrimSpace(strings.ReplaceAll(location, `\`, `/`))
	if location == "" {
		return ""
	}
	// URLs are not resource paths.
	if strings.Contains(location, "://") {
		return ""
	}
	location = path.Clean(location)
	if location == "." || location == "/" {
		return ""
	}
	if strings.HasPrefix(location, "../") || location == ".." {
		return ""
	}
	if !strings.HasPrefix(location, "/") {
		location = "/" + location
	}
	return location
}

// ResourcePathRelative returns the path relative to the path-mirror root
// (no leading slash), suitable for SafeJoin.
func ResourcePathRelative(canonical string) (string, bool) {
	canonical = NormalizeResourceLookupPath(canonical)
	if canonical == "" {
		return "", false
	}
	rel := strings.TrimPrefix(canonical, "/")
	if rel == "" || strings.Contains(rel, "..") {
		return "", false
	}
	return rel, true
}
