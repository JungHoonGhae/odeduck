package dataset

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/transform"
)

type ZIPMember struct {
	Name          string `json:"name"`
	NameEncoding  string `json:"nameEncoding"`
	RawNameSHA256 string `json:"rawNameSha256"`
	Bytes         uint64 `json:"bytes"`
	Format        string `json:"format"`
}
type ArchiveProvenance struct {
	Member        string `json:"member"`
	NameEncoding  string `json:"nameEncoding"`
	RawNameSHA256 string `json:"rawNameSha256"`
	MemberSHA256  string `json:"memberSha256"`
	MemberBytes   int64  `json:"memberBytes"`
}

// LayoutFile reuses the single goal layout action for two actual file formats.
// ZIP listing returns central-directory metadata only, never member values or
// proof that the named contents have passed decompression/checksum validation.
func (i *Inspector) LayoutFile(ctx context.Context, asset Asset, sheet string) (FileLayout, error) {
	if strings.EqualFold(path.Ext(asset.Name), ".xlsx") || strings.EqualFold(asset.Format, "XLSX") {
		return i.LayoutXLSX(ctx, asset, sheet)
	}
	if sheet != "" {
		return FileLayout{}, fmt.Errorf("ZIP layout does not accept a worksheet selector")
	}
	body, err := i.downloadSampleAsset(ctx, asset, "ZIP")
	if err != nil {
		return FileLayout{}, err
	}
	members, err := strictZIPMembers(body)
	if err != nil {
		return FileLayout{}, err
	}
	out := FileLayout{Format: "ZIP", SHA256: fmt.Sprintf("%x", sha256.Sum256(body)), Bytes: int64(len(body)), Warnings: []string{"ZIP central-directory metadata only; member names are untrusted source metadata. Listing is not checksum/schema validation, population coverage or current operational status. Select an exact CSV member; no recursive extraction or automatic concatenation."}}
	for name, m := range members {
		if !m.FileInfo().IsDir() {
			out.Members = append(out.Members, zipMemberMetadata(name, m))
		}
	}
	sort.Slice(out.Members, func(a, b int) bool { return out.Members[a].Name < out.Members[b].Name })
	return out, nil
}

func (i *Inspector) SampleZIPCSV(ctx context.Context, asset Asset, member string, limit int, where map[string]string) (TableSample, error) {
	if err := ValidateCSVSelection(where); err != nil {
		return TableSample{}, err
	}
	if limit < 1 || limit > 1000 {
		return TableSample{}, fmt.Errorf("sample row limit must be 1–1000")
	}
	if !safeZIPName(member) || !strings.EqualFold(path.Ext(member), ".csv") {
		return TableSample{}, fmt.Errorf("ZIP sampling requires an exact safe CSV member name")
	}
	body, err := i.downloadSampleAsset(ctx, asset, "ZIP")
	if err != nil {
		return TableSample{}, err
	}
	members, err := strictZIPMembers(body)
	if err != nil {
		return TableSample{}, err
	}
	m := members[member]
	if m == nil || m.FileInfo().IsDir() {
		return TableSample{}, fmt.Errorf("exact CSV member not found in archive")
	}
	const maxMember = 8 << 20
	if m.UncompressedSize64 > maxMember {
		return TableSample{}, fmt.Errorf("selected ZIP CSV exceeds 8 MiB")
	}
	rc, err := m.Open()
	if err != nil {
		return TableSample{}, err
	}
	contents, err := io.ReadAll(io.LimitReader(sampleContextReader{ctx: ctx, r: rc}, maxMember+1))
	closeErr := rc.Close()
	if err != nil {
		return TableSample{}, fmt.Errorf("ZIP CSV member read/checksum: %w", err)
	}
	if closeErr != nil {
		return TableSample{}, closeErr
	}
	if len(contents) > maxMember || uint64(len(contents)) != m.UncompressedSize64 {
		return TableSample{}, fmt.Errorf("ZIP CSV actual size exceeds limit or differs from directory")
	}
	s, err := sampleCSVBytes(ctx, contents, limit, where)
	if err != nil {
		return TableSample{}, err
	}
	s.Archive = &ArchiveProvenance{Member: member, RawNameSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(m.Name))), MemberSHA256: s.SHA256, MemberBytes: s.Bytes}
	s.Archive.NameEncoding = zipNameEncoding(m.Name)
	s.SHA256 = fmt.Sprintf("%x", sha256.Sum256(body))
	s.Bytes = int64(len(body))
	s.Warnings = []string{"One exact ZIP CSV member; original values and logical record positions retained. Whole selected member checksum was read before prefix selection. CSV scan/selection coverage concerns this member only, not other members or population completeness. No automatic concatenation, namespace or time approval."}
	return s, nil
}

func strictZIPMembers(body []byte) (map[string]*zip.File, error) {
	z, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, err
	}
	if len(z.File) > maxArchiveEntries {
		return nil, fmt.Errorf("ZIP exceeds 128 entries")
	}
	members := map[string]*zip.File{}
	var expanded uint64
	for _, m := range z.File {
		if m.UncompressedSize64 > maxExpandedBytes-expanded {
			return nil, fmt.Errorf("ZIP expanded size exceeds 64 MiB")
		}
		expanded += m.UncompressedSize64
		if m.Flags&1 != 0 {
			return nil, fmt.Errorf("encrypted ZIP entries are unsupported")
		}
		if !m.Mode().IsRegular() && !m.FileInfo().IsDir() {
			return nil, fmt.Errorf("ZIP links and special entries are unsupported")
		}
		name, err := exactZIPName(m.Name, m.NonUTF8)
		if err != nil {
			return nil, err
		}
		name = strings.TrimSuffix(name, "/")
		if !safeZIPName(name) || members[name] != nil {
			return nil, fmt.Errorf("unsafe or duplicate decoded ZIP member name")
		}
		members[name] = m
	}
	return members, nil
}

func safeZIPName(name string) bool {
	if name == "" || len(name) > 1024 || !utf8.ValidString(name) || path.Clean(name) != name || name == "." || name == ".." || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "../") || strings.ContainsAny(name, `\:`) {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func ValidateZIPCSVMember(name string) error {
	if !safeZIPName(name) || !strings.EqualFold(path.Ext(name), ".csv") {
		return fmt.Errorf("ZIP sampling requires an exact safe CSV member name")
	}
	return nil
}

func exactZIPName(raw string, nonUTF8 bool) (string, error) {
	if utf8.ValidString(raw) {
		return raw, nil
	}
	if !nonUTF8 {
		return "", fmt.Errorf("ZIP name marked UTF-8 contains invalid bytes")
	}
	name, _, err := transform.String(korean.EUCKR.NewDecoder(), raw)
	if err != nil {
		return "", fmt.Errorf("unsupported ZIP filename encoding")
	}
	encoded, _, err := transform.String(korean.EUCKR.NewEncoder(), name)
	if err != nil || encoded != raw {
		return "", fmt.Errorf("ZIP filename cannot be decoded losslessly as EUC-KR")
	}
	return name, nil
}

func zipMemberMetadata(name string, m *zip.File) ZIPMember {
	return ZIPMember{Name: name, NameEncoding: zipNameEncoding(m.Name), RawNameSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(m.Name))), Bytes: m.UncompressedSize64, Format: strings.ToUpper(strings.TrimPrefix(path.Ext(name), "."))}
}

func zipNameEncoding(raw string) string {
	if utf8.ValidString(raw) {
		return "utf-8"
	}
	return "euc-kr"
}
