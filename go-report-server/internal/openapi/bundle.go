package openapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// specFS is the embedded OpenAPI YAML tree (files under spec/).
var specFS = mustSubSpec()

func mustSubSpec() fs.FS {
	sub, err := fs.Sub(embeddedSpec, "spec")
	if err != nil {
		panic("openapi embed: " + err.Error())
	}
	return sub
}

// Path module filenames merged into the master document (order preserved for diffs).
var pathModules = []string{
	"auth.yaml",
	"reports.yaml",
	"execution.yaml",
	"parameters.yaml",
	"folders.yaml",
	"resources.yaml",
	"datasources.yaml",
	"embeddedmaps.yaml",
	"health.yaml",
	"users.yaml",
	"groups.yaml",
	"schedules.yaml",
	"jobs.yaml",
	"recipients.yaml",
}

type document map[string]any

var (
	bundleOnce sync.Once
	baseDoc    document
	bundleErr  error
)

// Bundle returns a deep-copied OpenAPI document with info.version set to appVersion.
func Bundle(appVersion string) (document, error) {
	bundleOnce.Do(func() {
		baseDoc, bundleErr = loadAndMerge()
	})
	if bundleErr != nil {
		return nil, bundleErr
	}
	out := deepCopyMap(baseDoc)
	if strings.TrimSpace(appVersion) == "" {
		appVersion = "0.1.0-foundation"
	}
	info, _ := out["info"].(map[string]any)
	if info == nil {
		info = map[string]any{}
		out["info"] = info
	}
	info["version"] = appVersion
	return out, nil
}

// MarshalYAML returns the bundled specification as YAML.
func MarshalYAML(appVersion string) ([]byte, error) {
	doc, err := Bundle(appVersion)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	_ = enc.Close()
	return buf.Bytes(), nil
}

// MarshalJSON returns the bundled specification as indented JSON.
func MarshalJSON(appVersion string) ([]byte, error) {
	doc, err := Bundle(appVersion)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(doc, "", "  ")
}

func loadAndMerge() (document, error) {
	raw, err := fs.ReadFile(specFS, "openapi.yaml")
	if err != nil {
		return nil, fmt.Errorf("read openapi.yaml: %w", err)
	}
	// Unmarshal into map[string]any (not document) so nested maps stay map[string]any.
	// yaml.v3 types nested maps as the named destination type when using `document`.
	var master map[string]any
	if err := yaml.Unmarshal(raw, &master); err != nil {
		return nil, fmt.Errorf("parse openapi.yaml: %w", err)
	}

	schemaRaw, err := fs.ReadFile(specFS, "schemas.yaml")
	if err != nil {
		return nil, fmt.Errorf("read schemas.yaml: %w", err)
	}
	var schemasDoc map[string]any
	if err := yaml.Unmarshal(schemaRaw, &schemasDoc); err != nil {
		return nil, fmt.Errorf("parse schemas.yaml: %w", err)
	}
	mergeComponents(master, schemasDoc)

	paths := asStringMap(master["paths"])
	if paths == nil {
		paths = map[string]any{}
		master["paths"] = paths
	}

	for _, name := range pathModules {
		modRaw, err := fs.ReadFile(specFS, name)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		var mod map[string]any
		if err := yaml.Unmarshal(modRaw, &mod); err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		modPaths := asStringMap(mod["paths"])
		for p, item := range modPaths {
			if _, exists := paths[p]; exists {
				return nil, fmt.Errorf("duplicate path %q in %s", p, name)
			}
			paths[p] = item
		}
		mergeComponents(master, mod)
	}

	if err := rewriteLocalRefs(master); err != nil {
		return nil, err
	}
	return master, nil
}

// asStringMap normalizes yaml/json map values to map[string]any.
func asStringMap(v any) map[string]any {
	switch t := v.(type) {
	case map[string]any:
		return t
	case document:
		return map[string]any(t)
	default:
		return nil
	}
}

func mergeComponents(dst, src map[string]any) {
	srcComp := asStringMap(src["components"])
	if len(srcComp) == 0 {
		return
	}
	dstComp := asStringMap(dst["components"])
	if dstComp == nil {
		dstComp = map[string]any{}
		dst["components"] = dstComp
	}
	for section, sectionVal := range srcComp {
		srcMap := asStringMap(sectionVal)
		if srcMap == nil {
			dstComp[section] = sectionVal
			continue
		}
		dstMap := asStringMap(dstComp[section])
		if dstMap == nil {
			dstMap = map[string]any{}
			dstComp[section] = dstMap
		}
		for k, v := range srcMap {
			dstMap[k] = v
		}
	}
}

// rewriteLocalRefs converts modular refs like "./schemas.yaml#/components/schemas/Error"
// into in-document refs "#/components/schemas/Error" after merge.
func rewriteLocalRefs(v any) error {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if k == "$ref" {
				s, ok := val.(string)
				if !ok {
					continue
				}
				if i := strings.Index(s, "#/"); i >= 0 {
					t[k] = s[i:]
				}
				continue
			}
			if err := rewriteLocalRefs(val); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range t {
			if err := rewriteLocalRefs(item); err != nil {
				return err
			}
		}
	}
	return nil
}

func deepCopyMap(in document) document {
	b, err := json.Marshal(in)
	if err != nil {
		out := document{}
		for k, v := range in {
			out[k] = v
		}
		return out
	}
	var out document
	_ = json.Unmarshal(b, &out)
	return out
}

// ValidateBasic checks required top-level OpenAPI fields and unique operationIds.
func ValidateBasic(doc document) error {
	if doc["openapi"] == nil {
		return fmt.Errorf("missing openapi version")
	}
	info, _ := doc["info"].(map[string]any)
	if info == nil || info["title"] == nil || info["version"] == nil {
		return fmt.Errorf("missing info.title or info.version")
	}
	paths := asStringMap(doc["paths"])
	if len(paths) == 0 {
		return fmt.Errorf("no paths defined")
	}
	seen := map[string]string{}
	var keys []string
	for p := range paths {
		keys = append(keys, p)
	}
	sort.Strings(keys)
	for _, p := range keys {
		item := asStringMap(paths[p])
		for _, method := range []string{"get", "post", "put", "patch", "delete", "head", "options", "trace"} {
			op := asStringMap(item[method])
			if op == nil {
				continue
			}
			id, _ := op["operationId"].(string)
			if id == "" {
				return fmt.Errorf("%s %s: missing operationId", strings.ToUpper(method), p)
			}
			if prev, ok := seen[id]; ok {
				return fmt.Errorf("duplicate operationId %q (%s and %s %s)", id, prev, strings.ToUpper(method), p)
			}
			seen[id] = strings.ToUpper(method) + " " + p
		}
	}
	return nil
}

// ListEmbedded returns embedded OpenAPI source filenames (for tests/diagnostics).
func ListEmbedded() ([]string, error) {
	var names []string
	err := fs.WalkDir(specFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".yaml") {
			names = append(names, path)
		}
		return nil
	})
	sort.Strings(names)
	return names, err
}
