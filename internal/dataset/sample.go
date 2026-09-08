package dataset

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/transform"
)

// TableSample is a bounded selection, not a random/population sample.
// SHA256 identifies the complete downloaded bytes, Rows only the selected cells.
// CSV fields remain strings, including empty strings; no null token is inferred.
type TableSample struct {
	Rows      []map[string]any
	SHA256    string
	Bytes     int64
	Prefix    bool
	Selection *SelectionReport
	Table     *TableProvenance
	Warnings  []string
	Archive   *ArchiveProvenance
	CSV       *CSVProvenance
	Document  *DocumentProvenance
}

// CSVProvenance counts logical data records after the header (starting at one).
// StartLines are physical CSV lines and may differ after quoted multiline cells.
type CSVProvenance struct {
	Encoding    string `json:"encoding"`
	DataRecords []int  `json:"dataRecords"`
	StartLines  []int  `json:"startLines"`
}

// SelectionReport covers only the downloaded asset scan, never the population
// represented by a dataset. A truncated scan counts one additional matching row.
type SelectionReport struct {
	Mode         string `json:"mode"`
	ScannedRows  int    `json:"scannedRows"`
	MatchedRows  int    `json:"matchedRows"`
	ReturnedRows int    `json:"returnedRows"`
	Exhausted    bool   `json:"exhausted"`
}

func ValidateCSVSelection(where map[string]string) error {
	if len(where) > 8 {
		return fmt.Errorf("CSV selection allows at most 8 exact string predicates")
	}
	for field, value := range where {
		if !validCSVSelectionTerm(field, value) {
			return fmt.Errorf("CSV selection needs nonblank field/value pairs of at most 256 bytes; null selection is unsupported")
		}
	}
	return nil
}

func validCSVSelectionTerm(field, value string) bool {
	return strings.TrimSpace(field) != "" && len(field) <= 256 && strings.TrimSpace(value) != "" && len(value) <= 256
}

// SampleCSV reuses inspected Asset retrieval. The caller must select the exact
// Asset from Inspect; model/user input must never construct its Request.
// Other formats remain schema-inspectable but cannot silently become CSV rows.
func (i *Inspector) SampleCSV(ctx context.Context, asset Asset, limit int) (TableSample, error) {
	return i.SampleCSVSelected(ctx, asset, limit, nil)
}

// SampleCSVSelected applies conjunctive exact string equality BEFORE the row
// cap. Selection happens locally in the bounded download, not in guessed portal
// query parameters. Neither original values nor namespace semantics are changed.
func (i *Inspector) SampleCSVSelected(ctx context.Context, asset Asset, limit int, where map[string]string) (TableSample, error) {
	if err := ValidateCSVSelection(where); err != nil {
		return TableSample{}, err
	}
	if limit < 1 || limit > 1000 {
		return TableSample{}, fmt.Errorf("sample row limit must be 1–1000")
	}
	body, err := i.downloadSampleAsset(ctx, asset, "CSV")
	if err != nil {
		return TableSample{}, err
	}
	return sampleCSVBytes(ctx, body, limit, where)
}

func sampleCSVBytes(ctx context.Context, body []byte, limit int, where map[string]string) (TableSample, error) {
	if err := ValidateCSVSelection(where); err != nil {
		return TableSample{}, err
	}
	if limit < 1 || limit > 1000 {
		return TableSample{}, fmt.Errorf("sample row limit must be 1–1000")
	}
	result := TableSample{SHA256: fmt.Sprintf("%x", sha256.Sum256(body)), Bytes: int64(len(body)), CSV: &CSVProvenance{Encoding: "utf-8"}}
	if !utf8.Valid(body) {
		result.CSV.Encoding = "euc-kr"
		decoded, _, decodeErr := transform.Bytes(korean.EUCKR.NewDecoder(), body)
		if decodeErr != nil {
			return TableSample{}, fmt.Errorf("CSV encoding: %w", decodeErr)
		}
		roundTrip, _, encodeErr := transform.Bytes(korean.EUCKR.NewEncoder(), decoded)
		if encodeErr != nil || !bytes.Equal(roundTrip, body) {
			return TableSample{}, fmt.Errorf("CSV encoding cannot be decoded losslessly as UTF-8 or EUC-KR")
		}
		body = decoded
	}
	body = bytes.TrimPrefix(body, []byte{0xef, 0xbb, 0xbf})
	r := csv.NewReader(bytes.NewReader(body))
	first := strings.SplitN(string(body), "\n", 2)[0]
	if strings.Count(first, "\t") > strings.Count(first, ",") {
		r.Comma = '\t'
	} else if strings.Count(first, "|") > strings.Count(first, ",") {
		r.Comma = '|'
	}
	header, err := r.Read()
	if err != nil {
		return TableSample{}, fmt.Errorf("CSV header: %w", err)
	}
	if len(header) > 256 {
		return TableSample{}, fmt.Errorf("CSV column limit exceeded")
	}
	seen := map[string]bool{}
	for i, h := range header {
		h = strings.TrimSpace(h)
		if h == "" || seen[h] {
			return TableSample{}, fmt.Errorf("CSV needs unique nonempty single-row column names")
		}
		seen[h] = true
		header[i] = h
	}
	for field := range where {
		if !seen[field] {
			return TableSample{}, fmt.Errorf("CSV selection field %q not in observed header", field)
		}
	}
	if len(where) > 0 {
		result.Selection = &SelectionReport{Mode: "exact_strings_v1"}
	}
	dataRecord := 0
	for {
		if err := ctx.Err(); err != nil {
			return TableSample{}, err
		}
		values, err := r.Read()
		if err == io.EOF {
			if result.Selection != nil {
				result.Selection.Exhausted = true
			}
			break
		}
		if err != nil {
			return TableSample{}, fmt.Errorf("CSV record: %w", err)
		}
		dataRecord++
		if result.Selection != nil {
			result.Selection.ScannedRows++
			matches := true
			for index, field := range header {
				if want, required := where[field]; required && values[index] != want {
					matches = false
					break
				}
			}
			if !matches {
				continue
			}
			result.Selection.MatchedRows++
		}
		if len(result.Rows) == limit {
			result.Prefix = true
			break
		}
		row := map[string]any{}
		for i, h := range header {
			row[h] = values[i]
		}
		result.Rows = append(result.Rows, row)
		line, _ := r.FieldPos(0)
		result.CSV.DataRecords = append(result.CSV.DataRecords, dataRecord)
		result.CSV.StartLines = append(result.CSV.StartLines, line)
	}
	if result.Selection != nil {
		result.Selection.ReturnedRows = len(result.Rows)
	}
	if len(result.Rows) == 0 {
		if result.Selection != nil {
			return TableSample{}, fmt.Errorf("no CSV rows matched exact selection after scanning %d asset rows; not population absence evidence", result.Selection.ScannedRows)
		}
		return TableSample{}, fmt.Errorf("CSV contains no data rows")
	}
	return result, nil
}
