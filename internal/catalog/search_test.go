package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSearcherOwnsSemanticLifecycle(t *testing.T) {
	for _, state := range []string{"used", "missing", "stale", "corrupt", "load-error", "embed-error", "disabled"} {
		t.Run(state, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("HOME", root)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, ".config"))
			t.Setenv("APPDATA", filepath.Join(root, "AppData"))
			cat := &Catalog{SyncedAt: time.Now(), Entries: []Entry{{PK: "jeju", Title: "제주 실종", SvcType: SvcFILE}}}
			embedder := fakeEmbedder{model: "test", fn: func(string) []float32 { return []float32{1, 0} }}
			if state != "missing" && state != "disabled" {
				idx, err := BuildSemanticIndex(context.Background(), cat, embedder, 1, nil)
				if err != nil {
					t.Fatal(err)
				}
				if err := idx.Save(); err != nil {
					t.Fatal(err)
				}
				path, err := semanticIndexPath()
				if err != nil {
					t.Fatal(err)
				}
				switch state {
				case "stale":
					cat.SyncedAt = cat.SyncedAt.Add(time.Hour)
				case "corrupt":
					if err := os.WriteFile(path, []byte("broken gob"), 0600); err != nil {
						t.Fatal(err)
					}
				case "load-error":
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(path, 0700); err != nil {
						t.Fatal(err)
					}
				case "embed-error":
					embedder.err = fmt.Errorf("embedder unavailable")
				}
			}
			searcher := Searcher{Embedder: embedder}
			for _, required := range []bool{false, true} {
				result, err := searcher.Search(context.Background(), cat, QueryPlan{Intent: "제주 실종", Limit: 1}, SearchOptions{Semantic: state != "disabled", RequireSemantic: required})
				if (err != nil) != (required && state != "used") {
					t.Fatalf("required=%v result=%+v err=%v", required, result, err)
				}
				if err != nil && !strings.Contains(err.Error(), "semantic-build") {
					t.Fatal(err)
				}
				if len(result.Hits) != 1 {
					t.Fatalf("diagnostic hits lost: %+v", result)
				}
				if state == "disabled" {
					if result.Semantic != nil {
						t.Fatalf("disabled: %+v", result.Semantic)
					}
					continue
				}
				want := SemanticUnavailable
				if state == "used" {
					want = SemanticUsed
				}
				if state == "missing" {
					want = SemanticNotIndexed
				}
				if result.Semantic == nil || result.Semantic.Status != want {
					t.Fatalf("want %s: %+v", want, result)
				}
				if (len(result.Warnings) > 0) != (state != "used") {
					t.Fatalf("warnings: %+v", result)
				}
			}
		})
	}
}

func TestSearcherUsesSuppliedIndexWithoutDisk(t *testing.T) {
	cat := &Catalog{Entries: []Entry{{PK: "1", Title: "제주 실종"}}}
	embedder := fakeEmbedder{model: "test", fn: func(string) []float32 { return []float32{1, 0} }}
	idx, err := BuildSemanticIndex(context.Background(), cat, embedder, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (Searcher{Index: idx, Embedder: embedder}).Search(context.Background(), cat, QueryPlan{Intent: "제주 실종"}, SearchOptions{Semantic: true, RequireSemantic: true})
	if err != nil || result.Semantic.Status != SemanticUsed {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestSearcherBuildsDefaultAdapterFromLoadedModel(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, ".config"))
	t.Setenv("APPDATA", filepath.Join(root, "AppData"))
	cat := &Catalog{Entries: []Entry{{PK: "jeju", Title: "제주 실종"}}}
	embedder := fakeEmbedder{model: "snapshot-specific-model", fn: func(string) []float32 { return []float32{1, 0} }}
	idx, err := BuildSemanticIndex(context.Background(), cat, embedder, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.Save(); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model string
			Input []string
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/embed" || request.Model != embedder.model || len(request.Input) != 1 {
			t.Errorf("unexpected embedding request: %s %s %+v", r.Method, r.URL.Path, request)
		}
		fmt.Fprint(w, `{"embeddings":[[1,0]]}`)
	}))
	defer srv.Close()
	t.Setenv("ODEDUCK_OLLAMA_URL", srv.URL)
	result, err := (Searcher{}).Search(context.Background(), cat, QueryPlan{Intent: "제주 실종"}, SearchOptions{Semantic: true, RequireSemantic: true})
	if err != nil || result.Semantic == nil || result.Semantic.Model != embedder.model {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}
