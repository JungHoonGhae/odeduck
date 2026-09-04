package apicall

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	MaxProfileFields = 8
	maxProfileValues = 50
	maxProfilePaths  = 8
)

// SampleProfile describes key evidence inside one bounded call response. It is
// intentionally per-response and stateless: callers compare two profiles from
// two call_api invocations without odeduck retaining public response data.
type SampleProfile struct {
	Fields       []FieldProfile `json:"fields"`
	EvidenceHash string         `json:"evidenceHash"`
}

// FieldProfile preserves raw string values so identifiers such as "00110" do
// not lose leading zeroes. Counts cover the complete bounded response even when
// Values is capped to keep MCP context small.
type FieldProfile struct {
	Field           string   `json:"field"`
	Matched         bool     `json:"matched"`
	Paths           []string `json:"paths,omitempty"`
	PathCount       int      `json:"pathCount,omitempty"`
	PathsTruncated  bool     `json:"pathsTruncated,omitempty"`
	Ambiguous       bool     `json:"ambiguous,omitempty"`
	Count           int      `json:"count"`
	NullCount       int      `json:"nullCount,omitempty"`
	DistinctCount   int      `json:"distinctCount"`
	DuplicateCount  int      `json:"duplicateCount,omitempty"`
	Values          []string `json:"values,omitempty"`
	ValuesTruncated bool     `json:"valuesTruncated,omitempty"`
	seen            map[string]struct{}
	pathSet         map[string]struct{}
}

// ProfileBody finds requested leaf fields recursively in JSON/XML-decoded
// response bodies. A selector without a dot matches a leaf field name at any
// depth; a dotted selector matches a full path or path suffix.
func ProfileBody(body any, fields []string) (*SampleProfile, error) {
	fields, err := normalizeProfileFields(fields)
	if err != nil {
		return nil, err
	}
	profile := &SampleProfile{Fields: make([]FieldProfile, len(fields))}
	for i, field := range fields {
		profile.Fields[i].Field = field
		profile.Fields[i].seen = map[string]struct{}{}
		profile.Fields[i].pathSet = map[string]struct{}{}
	}
	walkProfile(body, "", fields, profile.Fields)
	for i := range profile.Fields {
		item := &profile.Fields[i]
		item.PathCount = len(item.pathSet)
		item.PathsTruncated = item.PathCount > len(item.Paths)
		item.Matched = item.PathCount > 0
		item.DuplicateCount = item.Count - item.DistinctCount
		if item.PathCount > 1 {
			// Never merge identically named leaves from unrelated namespaces into
			// apparent join evidence. The caller must choose one surfaced dotted
			// path before treating the values as join evidence.
			item.Ambiguous = true
			item.Count, item.NullCount, item.DistinctCount, item.DuplicateCount = 0, 0, 0, 0
			item.Values = nil
			item.ValuesTruncated = false
		}
	}
	// The hash lets a durable connection assessment cite the exact bounded
	// profile without retaining its raw public-data values in the ledger.
	canonical, _ := json.Marshal(profile.Fields)
	profile.EvidenceHash = fmt.Sprintf("%x", sha256.Sum256(canonical))
	return profile, nil
}

func normalizeProfileFields(fields []string) ([]string, error) {
	if len(fields) > MaxProfileFields {
		return nil, fmt.Errorf("profile field는 최대 %d개입니다", MaxProfileFields)
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(fields))
	for _, raw := range fields {
		field := strings.TrimSpace(raw)
		if field == "" {
			return nil, fmt.Errorf("profile field가 비어 있습니다")
		}
		if len([]rune(field)) > 80 {
			return nil, fmt.Errorf("profile field가 너무 깁니다: %q", field)
		}
		key := strings.ToLower(field)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, field)
	}
	return out, nil
}

func walkProfile(value any, path string, fields []string, profiles []FieldProfile) {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			child := typed[key]
			for i, selector := range fields {
				if fieldMatches(selector, key, childPath) {
					addProfileValue(&profiles[i], childPath, child)
				}
			}
			walkProfile(child, childPath, fields, profiles)
		}
	case []any:
		for _, child := range typed {
			walkProfile(child, path, fields, profiles)
		}
	}
}

func fieldMatches(selector, leaf, path string) bool {
	if strings.Contains(selector, ".") {
		return strings.EqualFold(path, selector) ||
			strings.HasSuffix(strings.ToLower(path), "."+strings.ToLower(selector))
	}
	return strings.EqualFold(leaf, selector)
}

func addProfileValue(profile *FieldProfile, path string, value any) {
	if value == nil {
		recordProfilePath(profile, path)
		profile.NullCount++
		return
	}
	if _, nestedMap := value.(map[string]any); nestedMap {
		return
	}
	if nestedSlice, ok := value.([]any); ok {
		for _, child := range nestedSlice {
			addProfileValue(profile, path, child)
		}
		return
	}
	scalar, ok := profileScalar(value)
	if !ok {
		return
	}
	recordProfilePath(profile, path)
	profile.Count++
	if _, exists := profile.seen[scalar]; !exists {
		profile.seen[scalar] = struct{}{}
		profile.DistinctCount++
		if len(profile.Values) < maxProfileValues {
			profile.Values = append(profile.Values, scalar)
		} else {
			profile.ValuesTruncated = true
		}
	}
}

func recordProfilePath(profile *FieldProfile, path string) {
	if _, exists := profile.pathSet[path]; exists {
		return
	}
	profile.pathSet[path] = struct{}{}
	if len(profile.Paths) < maxProfilePaths {
		profile.Paths = append(profile.Paths, path)
	}
}

func profileScalar(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case json.Number:
		return typed.String(), true
	case float64:
		return strconv.FormatFloat(typed, 'g', -1, 64), true
	case float32:
		return strconv.FormatFloat(float64(typed), 'g', -1, 32), true
	case int:
		return strconv.Itoa(typed), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	case bool:
		return strconv.FormatBool(typed), true
	default:
		return "", false
	}
}
