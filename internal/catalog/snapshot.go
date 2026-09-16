package catalog

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const maxPrebuiltSnapshotBytes int64 = 256 << 20

// InstallResult explains whether a validated prebuilt snapshot replaced local
// state. Keeping a newer local snapshot is a successful no-op, not an error.
type InstallResult struct {
	Installed bool   `json:"installed"`
	Reason    string `json:"reason,omitempty"`
	Entries   int    `json:"entries"`
	Source    string `json:"source"`
	SyncedAt  string `json:"syncedAt"`
}

// InstallSnapshot validates and atomically installs a gzip-compressed Catalog.
// Release checksums authenticate the compressed artifact; this module owns the
// expanded-size bound, structural invariants and newer-local preservation.
func InstallSnapshot(compressed io.Reader) (InstallResult, error) {
	candidate, result, err := readPrebuiltSnapshot(compressed)
	if err != nil {
		return InstallResult{}, err
	}
	if current, err := Load(); err == nil && preserveLocalSnapshot(current, candidate) {
		result.Reason = "local_newer_or_complete"
		return result, nil
	}
	if err := candidate.Save(); err != nil {
		return InstallResult{}, err
	}
	result.Installed = true
	return result, nil
}

// ValidateSnapshot checks the exact compressed artifact without reading or
// changing local catalogue state. Installers use it before replacing binaries,
// preventing a malformed optional snapshot from causing a partial upgrade.
func ValidateSnapshot(compressed io.Reader) (InstallResult, error) {
	_, result, err := readPrebuiltSnapshot(compressed)
	if err != nil {
		return InstallResult{}, err
	}
	result.Reason = "validated"
	return result, nil
}

// ReadSnapshot validates and reads a packaged catalogue without accessing local
// state. Release checks use the candidate bytes, never an installed catalogue.
func ReadSnapshot(compressed io.Reader) (*Catalog, error) {
	candidate, _, err := readPrebuiltSnapshot(compressed)
	return candidate, err
}

func readPrebuiltSnapshot(compressed io.Reader) (*Catalog, InstallResult, error) {
	zr, err := gzip.NewReader(compressed)
	if err != nil {
		return nil, InstallResult{}, fmt.Errorf("prebuilt snapshot gzip 해석 실패: %w", err)
	}
	body, readErr := io.ReadAll(io.LimitReader(zr, maxPrebuiltSnapshotBytes+1))
	closeErr := zr.Close()
	if readErr != nil {
		return nil, InstallResult{}, fmt.Errorf("prebuilt snapshot 읽기 실패: %w", readErr)
	}
	if closeErr != nil {
		return nil, InstallResult{}, fmt.Errorf("prebuilt snapshot gzip 검증 실패: %w", closeErr)
	}
	if int64(len(body)) > maxPrebuiltSnapshotBytes {
		return nil, InstallResult{}, fmt.Errorf("prebuilt snapshot이 허용 크기 %d bytes를 초과했습니다", maxPrebuiltSnapshotBytes)
	}
	var candidate Catalog
	if err := json.Unmarshal(body, &candidate); err != nil {
		return nil, InstallResult{}, fmt.Errorf("prebuilt snapshot JSON 해석 실패: %w", err)
	}
	if err := validatePrebuilt(&candidate); err != nil {
		return nil, InstallResult{}, err
	}
	result := InstallResult{
		Entries: len(candidate.Entries), Source: candidate.Source,
		SyncedAt: candidate.SyncedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
	return &candidate, result, nil
}

func preserveLocalSnapshot(current, candidate *Catalog) bool {
	if current == nil || candidate == nil || candidate.SyncedAt.After(current.SyncedAt) {
		return false
	}
	// A newer narrow or fallback-built snapshot must not block an official ALL
	// snapshot during upgrade: it lacks either part of the search space or the
	// operation provenance bundled for API-first inspection.
	if strings.ToUpper(strings.TrimSpace(current.Type)) != "ALL" {
		return false
	}
	if sourceRank(candidate.Source) > sourceRank(current.Source) {
		return false
	}
	if len(current.Entries) < len(candidate.Entries) || officialContractCount(current) < officialContractCount(candidate) {
		return false
	}
	return true
}

func sourceRank(source string) int {
	switch source {
	case SourceOfficial:
		return 3
	case SourceCombined:
		return 2
	case SourceOfficialFile:
		return 1
	default:
		return 0
	}
}

func officialContractCount(candidate *Catalog) int {
	count := 0
	if candidate == nil {
		return count
	}
	for _, entry := range candidate.Entries {
		if entry.OfficialAPI != nil {
			count++
		}
	}
	return count
}

func validatePrebuilt(candidate *Catalog) error {
	if candidate.Source != SourceOfficial && candidate.Source != SourceCombined && candidate.Source != SourceOfficialFile {
		return fmt.Errorf("prebuilt snapshot source가 %q입니다 — 검증된 official 계열만 설치할 수 있습니다", candidate.Source)
	}
	if strings.ToUpper(strings.TrimSpace(candidate.Type)) != "ALL" {
		return fmt.Errorf("prebuilt snapshot 유형이 %q입니다 — ALL이 필요합니다", candidate.Type)
	}
	if candidate.SyncedAt.IsZero() || len(candidate.Entries) == 0 {
		return fmt.Errorf("prebuilt snapshot에 수집시각 또는 데이터 노드가 없습니다")
	}
	seen := make(map[string]bool, len(candidate.Entries))
	for _, entry := range candidate.Entries {
		pk := strings.TrimSpace(entry.PK)
		if pk == "" || strings.TrimSpace(entry.Title) == "" {
			return fmt.Errorf("prebuilt snapshot에 식별자 또는 제목이 없는 노드가 있습니다")
		}
		if seen[pk] {
			return fmt.Errorf("prebuilt snapshot에 중복 PK %q가 있습니다", pk)
		}
		seen[pk] = true
		switch entry.SvcType {
		case "", SvcREST, SvcLINK, SvcFILE, SvcSTD:
		default:
			return fmt.Errorf("prebuilt snapshot PK %s의 유형 %q가 유효하지 않습니다", pk, entry.SvcType)
		}
	}
	return nil
}
