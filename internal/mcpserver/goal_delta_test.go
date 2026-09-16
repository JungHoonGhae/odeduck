package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func decodeExact(t *testing.T, b []byte) any {
	t.Helper()
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var x any
	if err := d.Decode(&x); err != nil {
		t.Fatal(err)
	}
	return x
}

// Test-side RFC 6902 application checks the wire contract independently of diff.
func applyGoalChange(t *testing.T, root any, c goalChange) any {
	t.Helper()
	if c.Path == "" {
		return decodeExact(t, c.Value)
	}
	path := strings.Split(strings.TrimPrefix(c.Path, "/"), "/")
	for i := range path {
		path[i] = strings.ReplaceAll(strings.ReplaceAll(path[i], "~1", "/"), "~0", "~")
	}
	var apply func(any, []string) any
	apply = func(v any, p []string) any {
		leaf := len(p) == 1
		switch x := v.(type) {
		case map[string]any:
			if leaf {
				if c.Op == "remove" {
					delete(x, p[0])
				} else {
					x[p[0]] = decodeExact(t, c.Value)
				}
			} else {
				x[p[0]] = apply(x[p[0]], p[1:])
			}
			return x
		case []any:
			i := len(x)
			if p[0] != "-" {
				var err error
				i, err = strconv.Atoi(p[0])
				if err != nil {
					t.Fatal(err)
				}
			}
			if leaf {
				switch c.Op {
				case "add":
					x = append(x, nil)
					copy(x[i+1:], x[i:])
					x[i] = decodeExact(t, c.Value)
				case "remove":
					x = append(x[:i], x[i+1:]...)
				case "replace":
					x[i] = decodeExact(t, c.Value)
				}
			} else {
				x[i] = apply(x[i], p[1:])
			}
			return x
		default:
			t.Fatalf("bad pointer %q on %T", c.Path, v)
		}
		return nil
	}
	return apply(root, path)
}

func TestGoalPatchPreservesNumbersMissingNullAndEscapedPaths(t *testing.T) {
	before := json.RawMessage(`{"a/b~c":[{"n":9007199254740993,"null":null,"delete":false},0,2],"kept":true}`)
	after := json.RawMessage(`{"a/b~c":[{"n":9007199254740994,"null":false,"new":null}],"kept":true,"appended":[0]}`)
	var changes []goalChange
	goalDiff("", before, after, &changes)
	got := decodeExact(t, before)
	for _, c := range changes {
		got = applyGoalChange(t, got, c)
	}
	if !reflect.DeepEqual(got, decodeExact(t, after)) {
		t.Fatalf("lossy patch: %+v", got)
	}
}

func TestDefaultMCPDeltaReconstructsSameComputedArtifactWithoutChildModels(t *testing.T) {
	deps := goalwork.Dependencies{
		Search: func(context.Context, string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: "records", Title: "source"}}}, nil
		},
		Inspect: func(context.Context, string) (goalwork.Inspection, error) {
			return goalwork.Inspection{PK: "records"}, nil
		},
		Sample: func(context.Context, goalwork.SampleRequest, goalwork.Inspection) (goalwork.Acquired, error) {
			return goalwork.Acquired{Delivery: "REST", Rows: []goalwork.Row{{"amount": "9007199254740993"}, {"amount": "1"}}}, nil
		},
		Review: func(context.Context, goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
			t.Fatal("default host workflow invoked another model")
			return goalwork.ReviewAssessment{}, nil
		},
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	registerGoalTool(s, goalwork.Policy{}, func(goalwork.Policy) goalwork.Dependencies { return deps })
	client := connectTestClient(t, s)
	call := func(args map[string]any) []byte {
		t.Helper()
		r, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "goal", Arguments: args})
		if err != nil || r.IsError {
			t.Fatalf("%v %+v", err, r)
		}
		return []byte(r.Content[0].(*mcp.TextContent).Text)
	}
	var wire struct {
		SessionID    string          `json:"sessionId"`
		State        json.RawMessage `json:"state"`
		Changes      []goalChange    `json:"changes"`
		Update       string          `json:"update"`
		BaseRevision int             `json:"baseRevision"`
	}
	initial := call(map[string]any{"goal": "sum the source sample", "requireSemantic": false})
	json.Unmarshal(initial, &wire)
	state := decodeExact(t, wire.State)
	id := wire.SessionID
	contract := goalwork.GoalContract{Outcome: "sum", Region: "fixture", Period: "source", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "records"}}, Outputs: []goalwork.OutputRequirement{{ID: "total", Role: "r", Type: "number", Description: "sum"}}}
	recipe := goalwork.Composition{ID: "sum", Purpose: "source sum", Base: "o1", Measures: []goalwork.Measure{{As: "n", Field: "o1.amount", Format: "decimal_v1", Unit: "fixture"}}, Aggregates: []goalwork.Aggregate{{As: "total", Op: "sum", Field: "n"}}, Roles: []goalwork.RoleBinding{{Role: "r", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "total", Field: "total"}}, Assumptions: []string{"source meaning remains unverified"}}
	decisions := []goalwork.Decision{{Action: "define", Contract: &contract}, {Action: "search", Query: "records", Role: "r"}, {Action: "inspect", PK: "records"}, {Action: "sample", Sample: &goalwork.SampleRequest{PK: "records", Delivery: "api"}}, {Action: "compose", Composition: &recipe}, {Action: "execute", CompositionID: "sum"}}
	deltaBytes, fullBytes := len(initial), len(initial)
	for i, d := range decisions {
		data := call(map[string]any{"sessionId": id, "revision": i, "decision": d})
		deltaBytes += len(data)
		wire.Changes = nil
		json.Unmarshal(data, &wire)
		if wire.Update != "delta" || wire.BaseRevision != i {
			t.Fatal("delta revision mismatch")
		}
		for _, change := range wire.Changes {
			state = applyGoalChange(t, state, change)
		}
		full := call(map[string]any{"sessionId": id, "fullState": true})
		fullBytes += len(full)
		var snapshot struct {
			State json.RawMessage `json:"state"`
		}
		json.Unmarshal(full, &snapshot)
		if !reflect.DeepEqual(state, decodeExact(t, snapshot.State)) {
			t.Fatalf("delta lost state at %s", d.Action)
		}
	}
	encoded, _ := json.Marshal(state)
	var view goalwork.View
	d := json.NewDecoder(bytes.NewReader(encoded))
	d.UseNumber()
	d.Decode(&view)
	if view.Status != "review_required" || view.Artifact.Rows[0]["total"] != json.Number("9007199254740994") || view.ModelUsage.Calls != 0 || view.Evaluation.Review != nil {
		t.Fatal("host path altered exact computation or claimed independent review")
	}
	if deltaBytes*100 >= fullBytes*80 {
		t.Fatalf("wire regression: delta=%d snapshot=%d", deltaBytes, fullBytes)
	}
	t.Logf("same 6-action fixture: delta=%d bytes snapshot=%d bytes reduction=%.1f%%; child models=0", deltaBytes, fullBytes, 100*(1-float64(deltaBytes)/float64(fullBytes)))
}
