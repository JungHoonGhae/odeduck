package mcpserver

import (
	"bytes"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// Goal changes are RFC 6902 operations relative to the last requested revision.
// Values remain raw JSON, preserving integer/decimal lexemes and null vs missing.
type goalChange struct {
	Op    string          `json:"op"`
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value,omitempty"`
}

func goalResponse(out *goalOut, full bool) ([]byte, error) {
	snapshot, err := json.Marshal(out.State)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"sessionId": out.SessionID, "state": json.RawMessage(snapshot), "update": "snapshot"}
	if out.Guide != "" {
		body["guide"] = out.Guide
	}
	if !full && out.previous != nil && out.State.Status != "expired" {
		prior, err := json.Marshal(out.previous)
		if err != nil {
			return nil, err
		}
		changes := []goalChange{}
		goalDiff("", prior, snapshot, &changes)
		body["state"] = map[string]any{"revision": out.State.Revision, "status": out.State.Status, "budget": out.State.Budget}
		body["update"] = "delta"
		body["baseRevision"] = out.previous.Revision
		body["changes"] = changes
	}
	return json.Marshal(body)
}

func goalDiff(path string, before, after json.RawMessage, changes *[]goalChange) {
	if bytes.Equal(before, after) {
		return
	}
	if len(before) > 0 && len(after) > 0 && before[0] == '{' && after[0] == '{' {
		var a, b map[string]json.RawMessage
		_ = json.Unmarshal(before, &a)
		_ = json.Unmarshal(after, &b)
		keys := make([]string, 0, len(a)+len(b))
		seen := map[string]bool{}
		for k := range a {
			keys = append(keys, k)
			seen[k] = true
		}
		for k := range b {
			if !seen[k] {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			p := path + "/" + strings.ReplaceAll(strings.ReplaceAll(k, "~", "~0"), "/", "~1")
			av, old := a[k]
			bv, current := b[k]
			switch {
			case !current:
				*changes = append(*changes, goalChange{Op: "remove", Path: p})
			case !old:
				*changes = append(*changes, goalChange{Op: "add", Path: p, Value: bv})
			default:
				goalDiff(p, av, bv, changes)
			}
		}
		return
	}
	if len(before) > 0 && len(after) > 0 && before[0] == '[' && after[0] == '[' {
		var a, b []json.RawMessage
		_ = json.Unmarshal(before, &a)
		_ = json.Unmarshal(after, &b)
		for i := 0; i < len(a) && i < len(b); i++ {
			goalDiff(path+"/"+strconv.Itoa(i), a[i], b[i], changes)
		}
		for i := len(a) - 1; i >= len(b); i-- {
			*changes = append(*changes, goalChange{Op: "remove", Path: path + "/" + strconv.Itoa(i)})
		}
		for i := len(a); i < len(b); i++ {
			*changes = append(*changes, goalChange{Op: "add", Path: path + "/-", Value: b[i]})
		}
		return
	}
	*changes = append(*changes, goalChange{Op: "replace", Path: path, Value: after})
}
