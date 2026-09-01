//go:build ignore

// Command sync-brand keeps the generated README brand block in sync with
// docs/brand/brand.json. Run it from any directory inside the repository:
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
	DisplayName   string `json:"displayName"`
	Tagline       string `json:"tagline"`
	Proofline     string `json:"proofline"`
	LogoPath      string `json:"logoPath"`
	LogoAlt       string `json:"logoAlt"`
	TechnicalName string `json:"technicalName"`
	Command       string `json:"command"`
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
	current, err := os.ReadFile(readmePath)
	if err != nil {
		fatal(err)
	}
	updated, err := replaceBlock(current, render(configuration))
	if err != nil {
		fatal(err)
	}
	if bytes.Equal(current, updated) {
		return
	}
	if *check {
		fatal(errors.New("README.md brand block is stale; run go run ./scripts/sync-brand.go"))
	}
	if err := os.WriteFile(readmePath, updated, 0o644); err != nil {
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
		"displayName":   configuration.DisplayName,
		"tagline":       configuration.Tagline,
		"proofline":     configuration.Proofline,
		"logoPath":      configuration.LogoPath,
		"logoAlt":       configuration.LogoAlt,
		"technicalName": configuration.TechnicalName,
		"command":       configuration.Command,
	}
	for name, value := range fields {
		if strings.TrimSpace(value) == "" {
			return brand{}, fmt.Errorf("brand field %q is required", name)
		}
	}
	return configuration, nil
}

func render(configuration brand) []byte {
	escape := html.EscapeString
	return []byte(fmt.Sprintf(`%s
<p align="center">
  <img src="%s" width="190" alt="%s">
</p>

<h1 align="center">%s</h1>

<p align="center"><em>%s</em></p>
<p align="center">%s</p>
%s`,
		startMarker,
		escape(configuration.LogoPath),
		escape(configuration.LogoAlt),
		escape(configuration.DisplayName),
		escape(configuration.Tagline),
		escape(configuration.Proofline),
		endMarker,
	))
}

func replaceBlock(current, generated []byte) ([]byte, error) {
	start := bytes.Index(current, []byte(startMarker))
	end := bytes.Index(current, []byte(endMarker))
	if start < 0 || end < 0 || end < start {
		return nil, fmt.Errorf("README.md must contain one ordered %s / %s block", startMarker, endMarker)
	}
	end += len(endMarker)
	if bytes.Contains(current[end:], []byte(startMarker)) || bytes.Contains(current[end:], []byte(endMarker)) {
		return nil, errors.New("README.md contains duplicate brand markers")
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
