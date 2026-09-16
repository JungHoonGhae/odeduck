//go:build ignore

// Command sync-brand keeps the README brand block and technical identity in
// sync with docs/brand/brand.json. Run it from any directory in the repository:
//
//	go run ./scripts/sync-brand.go
//	go run ./scripts/sync-brand.go --check
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
)

const (
	startMarker = "<!-- brand:start -->"
	endMarker   = "<!-- brand:end -->"
)

type brand struct {
	DisplayName     string   `json:"displayName"`
	Positioning     string   `json:"positioning"`
	Summary         string   `json:"summary"`
	Tagline         string   `json:"tagline"`
	Introduction    []string `json:"introduction"`
	LogoPath        string   `json:"logoPath"`
	LogoAlt         string   `json:"logoAlt"`
	TechnicalName   string   `json:"technicalName"`
	Command         string   `json:"command"`
	Repository      string   `json:"repository"`
	ModulePath      string   `json:"modulePath"`
	ConfigDirectory string   `json:"configDirectory"`
}

func main() {
	check := flag.Bool("check", false, "fail when README.md is not synchronized")
	flag.Parse()

	root, err := repositoryRoot()
	if err != nil {
		fatal(err)
	}
	configuration, err := loadBrand(filepath.Join(root, "docs", "brand", "brand.json"))
	if err != nil {
		fatal(err)
	}

	readmePath := filepath.Join(root, "README.md")
	if err := syncBlock(readmePath, startMarker, endMarker, render(configuration), *check); err != nil {
		fatal(err)
	}
	if err := validateProjectIdentity(root, configuration); err != nil {
		fatal(err)
	}
}

func repositoryRoot() (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory, nil
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", errors.New("go.mod not found in this directory or its parents")
		}
		directory = parent
	}
}

func loadBrand(path string) (brand, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return brand{}, err
	}
	var configuration brand
	if err := json.Unmarshal(content, &configuration); err != nil {
		return brand{}, fmt.Errorf("decode %s: %w", path, err)
	}
	fields := map[string]string{
		"displayName":     configuration.DisplayName,
		"positioning":     configuration.Positioning,
		"summary":         configuration.Summary,
		"tagline":         configuration.Tagline,
		"logoPath":        configuration.LogoPath,
		"logoAlt":         configuration.LogoAlt,
		"technicalName":   configuration.TechnicalName,
		"command":         configuration.Command,
		"repository":      configuration.Repository,
		"modulePath":      configuration.ModulePath,
		"configDirectory": configuration.ConfigDirectory,
	}
	for name, value := range fields {
		if strings.TrimSpace(value) == "" {
			return brand{}, fmt.Errorf("brand field %q is required", name)
		}
	}
	if len(configuration.Introduction) == 0 {
		return brand{}, errors.New("brand introduction is required")
	}
	for _, line := range configuration.Introduction {
		if strings.TrimSpace(line) == "" {
			return brand{}, errors.New("each introduction line requires nonempty text")
		}
	}
	return configuration, nil
}

func validateProjectIdentity(root string, configuration brand) error {
	checks := map[string][]string{
		"go.mod":                      {"module " + configuration.ModulePath},
		".goreleaser.yaml":            {"project_name: " + configuration.TechnicalName, "binary: " + configuration.Command},
		"install.sh":                  {`REPO="` + configuration.Repository + `"`, `BINARY="` + configuration.Command + `"`},
		"install.ps1":                 {`$Repo = "` + configuration.Repository + `"`},
		"README.md":                   {"https://github.com/" + configuration.Repository, "`" + configuration.Command + "`"},
		"internal/portal/daemon.go":   {"configDirName", `= "` + configuration.ConfigDirectory + `"`},
		"internal/version/version.go": {`CommandName = "` + configuration.Command + `"`},
	}
	for relative, required := range checks {
		content, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil {
			return err
		}
		for _, needle := range required {
			if !bytes.Contains(content, []byte(needle)) {
				return fmt.Errorf("%s is stale: missing %q from docs/brand/brand.json", relative, needle)
			}
		}
	}
	return nil
}

func syncBlock(path, start, end string, generated []byte, check bool) error {
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	updated, err := replaceMarkedBlock(current, []byte(start), []byte(end), generated)
	if err != nil {
		return err
	}
	if bytes.Equal(current, updated) {
		return nil
	}
	if check {
		return fmt.Errorf("%s is stale; run go run ./scripts/sync-brand.go", filepath.Base(path))
	}
	return os.WriteFile(path, updated, 0o644)
}

func render(configuration brand) []byte {
	escape := html.EscapeString
	lines := make([]string, 0, len(configuration.Introduction))
	for _, line := range configuration.Introduction {
		lines = append(lines, "  "+escape(line))
	}
	return []byte(fmt.Sprintf(`%s
<p align="center">
  <img src="%s" width="140" alt="%s">
</p>

<h1 align="center">%s · %s</h1>

<p align="center"><strong>%s</strong><br>
  %s
</p>

<p align="center">
%s
</p>

<p align="center"><strong>%s</strong></p>
%s`,
		startMarker,
		escape(configuration.LogoPath),
		escape(configuration.LogoAlt),
		escape(configuration.DisplayName),
		escape(configuration.TechnicalName),
		escape(configuration.Positioning),
		escape(configuration.Summary),
		strings.Join(lines, "<br>\n"),
		escape(configuration.Tagline),
		endMarker,
	))
}

func replaceBlock(current, generated []byte) ([]byte, error) {
	return replaceMarkedBlock(current, []byte(startMarker), []byte(endMarker), generated)
}

func replaceMarkedBlock(current, startMarkerBytes, endMarkerBytes, generated []byte) ([]byte, error) {
	start := bytes.Index(current, startMarkerBytes)
	end := bytes.Index(current, endMarkerBytes)
	if start < 0 || end < 0 || end < start {
		return nil, errors.New("document must contain one ordered generated block")
	}
	end += len(endMarkerBytes)
	if bytes.Contains(current[end:], startMarkerBytes) || bytes.Contains(current[end:], endMarkerBytes) {
		return nil, errors.New("document contains duplicate generated markers")
	}

	updated := make([]byte, 0, len(current)-end+start+len(generated))
	updated = append(updated, current[:start]...)
	updated = append(updated, generated...)
	updated = append(updated, current[end:]...)
	return updated, nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "sync-brand:", err)
	os.Exit(1)
}
