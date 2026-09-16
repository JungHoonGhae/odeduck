package scripts_test

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPrepareReleaseCatalogue(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("release packaging runs on Linux; CLI snapshot validation is tested on Windows")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Fatal("release script tests require jq")
	}
	var raw bytes.Buffer
	raw.WriteString(`{"source":"official-file+web","type":"ALL","syncedAt":"2026-09-09T00:00:00Z","entries":[`)
	for i := 0; i < 90001; i++ {
		if i > 0 {
			raw.WriteByte(',')
		}
		svc := "FILE"
		if i < 7000 {
			svc = "REST"
		} else if i < 11000 {
			svc = "LINK"
		}
		fmt.Fprintf(&raw, `{"pk":"%d","title":"fixture","dataTypes":["API","FILE"],"svcType":%q,"officialApi":{}}`, i, svc)
	}
	raw.WriteString(`]}`)
	var archive bytes.Buffer
	zw := gzip.NewWriter(&archive)
	if _, err := zw.Write(raw.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	asset := "odeduck-catalog.json.gz"
	checksum := fmt.Sprintf("%x  %s\n", sha256.Sum256(archive.Bytes()), asset)
	releases := `[
		{"tag_name":"v0.19.0","draft":false,"prerelease":false,"published_at":"2026-09-09T00:00:00Z"},
		{"tag_name":"catalog-20260910-123-1","draft":false,"prerelease":true,"published_at":"2026-09-10T00:00:00Z"},
		{"tag_name":"v0.20.0","draft":false,"prerelease":false,"published_at":"2026-09-11T00:00:00Z"},
		{"tag_name":"v0.21.0-rc.1","draft":false,"prerelease":true,"published_at":"2026-09-12T00:00:00Z"},
		{"tag_name":"catalog-20260913-124-1","draft":true,"prerelease":true,"published_at":"2026-09-13T00:00:00Z"}
	]`
	for _, tc := range []struct {
		name, pin, mode, checksums, wantTag, wantError string
		releases                                       string
		lowCoverage, existing                          bool
	}{
		{name: "reuse newest published catalogue", wantTag: "catalog-20260910-123-1"},
		{name: "bootstrap from application release", releases: `[ {"tag_name":"v0.19.0","draft":false,"prerelease":false,"published_at":"2026-09-09T00:00:00Z"} ]`, wantTag: "v0.19.0"},
		{name: "explicit pinned source", pin: "v0.19.0", wantTag: "v0.19.0"},
		{name: "catalogue refresh preferred to newer app packaging", releases: `[
			{"tag_name":"catalog-20260910-123-1","draft":false,"prerelease":true,"published_at":"2026-09-10T00:00:00Z"},
			{"tag_name":"v0.19.1","draft":false,"prerelease":false,"published_at":"2026-09-11T00:00:00Z"}
		]`, wantTag: "catalog-20260910-123-1"},
		{name: "paginated releases", releases: `[{"tag_name":"v0.19.0","draft":false,"prerelease":false,"published_at":"2026-09-09T00:00:00Z"}]
		[{"tag_name":"catalog-20260910-123-1","draft":false,"prerelease":true,"published_at":"2026-09-10T00:00:00Z"}]`, wantTag: "catalog-20260910-123-1"},
		{name: "missing source", releases: `[]`, wantError: "no published catalogue source"},
		{name: "draft cannot be pinned", pin: "catalog-20260913-124-1", wantError: "no published catalogue source"},
		{name: "download failure", mode: "download", wantError: "fixture download failure"},
		{name: "checksum mismatch", checksums: strings.Repeat("0", 64) + "  " + asset + "\n", wantError: "checksum mismatch"},
		{name: "missing checksum", checksums: strings.Repeat("0", 64) + "  unrelated.gz\n", wantError: "invalid catalogue checksum"},
		{name: "duplicate checksum", checksums: checksum + checksum, wantError: "invalid catalogue checksum"},
		{name: "malformed checksum", checksums: strings.Repeat("x", 64) + "  " + asset + "\n", wantError: "invalid catalogue checksum"},
		{name: "corrupt snapshot rejected", mode: "structure", wantError: "fixture structure failure"},
		{name: "search regression rejected", mode: "quality", wantError: "fixture quality failure"},
		{name: "coverage regression rejected", lowCoverage: true, wantError: "catalogue coverage regression"},
		{name: "existing output preserved", existing: true, wantError: "already exists"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			write := func(name string, body []byte, mode os.FileMode) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, name), body, mode); err != nil {
					t.Fatal(err)
				}
			}
			data := archive.Bytes()
			if tc.lowCoverage {
				var small bytes.Buffer
				w := gzip.NewWriter(&small)
				_, _ = w.Write([]byte(`{"source":"official-file+web","type":"ALL","entries":[]}`))
				_ = w.Close()
				data = small.Bytes()
			}
			write(asset, data, 0600)
			manifest := tc.checksums
			if manifest == "" {
				manifest = fmt.Sprintf("%x  %s\n", sha256.Sum256(data), asset)
			}
			write("checksums.txt", []byte(manifest), 0600)
			listing := tc.releases
			if listing == "" {
				listing = releases
			}
			write("releases.json", []byte(listing), 0600)
			write("gh", []byte(`#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$CATALOG_TEST_DIR/commands"
case "$1" in
api) cat "$CATALOG_TEST_DIR/releases.json" ;;
release)
  [ "$2" = download ] || exit 90
  if [ "$CATALOG_TEST_MODE" = download ]; then echo 'fixture download failure' >&2; exit 1; fi
  while [ "$#" -gt 0 ]; do
    if [ "$1" = --dir ]; then destination=$2; shift 2; else shift; fi
  done
  cp "$CATALOG_TEST_DIR/odeduck-catalog.json.gz" "$CATALOG_TEST_DIR/checksums.txt" "$destination/"
  ;;
*) exit 91 ;;
esac
`), 0700)
			write("odeduck", []byte(`#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$CATALOG_TEST_DIR/commands"
case "$*" in
'catalog install-snapshot '*)
  case "$*" in *--check-only*) ;; *) exit 92 ;; esac
  if [ "$CATALOG_TEST_MODE" = structure ]; then echo 'fixture structure failure' >&2; exit 1; fi
  echo '{"reason":"validated"}' ;;
'catalog validate-release --snapshot '*)
  if [ "$CATALOG_TEST_MODE" = quality ]; then echo 'fixture quality failure' >&2; exit 1; fi
  echo '{"passed":true}' ;;
*) echo 'unexpected catalogue action' >&2; exit 93 ;;
esac
`), 0700)
			out := filepath.Join(dir, "output")
			if tc.existing {
				if err := os.Mkdir(out, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(out, asset), []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command("sh", "./prepare-release-catalog.sh", filepath.Join(dir, "odeduck"), out, tc.pin)
			cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "CATALOG_TEST_DIR="+dir, "CATALOG_TEST_MODE="+tc.mode, "GITHUB_REF_NAME=v0.20.0")
			output, err := cmd.CombinedOutput()
			if tc.wantError != "" {
				if err == nil || !strings.Contains(string(output), tc.wantError) {
					t.Fatalf("want %q, got error=%v output=%s", tc.wantError, err, output)
				}
				if tc.existing {
					got, _ := os.ReadFile(filepath.Join(out, asset))
					if string(got) != "preserve" {
						t.Fatal("existing catalogue changed")
					}
				} else if _, err := os.Stat(filepath.Join(out, asset)); !os.IsNotExist(err) {
					t.Fatal("rejected catalogue was published to output")
				}
				return
			}
			if err != nil {
				t.Fatalf("prepare: %v\n%s", err, output)
			}
			got, err := os.ReadFile(filepath.Join(out, asset))
			if err != nil || !bytes.Equal(got, data) {
				t.Fatalf("catalogue bytes changed: %v", err)
			}
			var provenance struct {
				SourceTag string `json:"sourceTag"`
			}
			metadata, err := os.ReadFile(filepath.Join(out, "catalog-source.json"))
			if err != nil || json.Unmarshal(metadata, &provenance) != nil || provenance.SourceTag != tc.wantTag {
				t.Fatalf("source provenance = %s, error=%v", metadata, err)
			}
			commands, _ := os.ReadFile(filepath.Join(dir, "commands"))
			if strings.Contains(string(commands), "catalog sync") || strings.Contains(string(commands), "semantic-build") {
				t.Fatal("release reuse must not recollect or embed")
			}
		})
	}
}
