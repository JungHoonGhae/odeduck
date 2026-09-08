package dataset

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/transform"
)

const maxCSVScanBytes = 64 << 20

type CSVScanRecord struct {
	DataRecord int
	StartLine  int
	Values     map[string]string
}
type CSVScanReport struct {
	Encoding    string   `json:"encoding"`
	SHA256      string   `json:"sha256,omitempty"`
	Bytes       int64    `json:"bytes"`
	Columns     []string `json:"columns"`
	ScannedRows int      `json:"scannedRows"`
	MatchedRows int      `json:"matchedRows"`
	Exhausted   bool     `json:"exhausted"`
}

// ScanCSV consumes an inspected direct comma-delimited CSV once without
// retaining the whole asset. Visitors are trusted local consumers, never model
// code. Their interim state MUST be discarded on error: only successful EOF
// supplies a complete source hash and structural scan evidence. Matching is
// exact string equality, not verified geographic scope or population coverage.
// The first physical header line selects UTF-8 or strict EUC-KR. An ASCII header
// followed by EUC-KR values fails as invalid UTF-8; there is no midstream fallback.
func (i *Inspector) ScanCSV(ctx context.Context, asset Asset, selection CSVSelection, visit func(CSVScanRecord) error) (report CSVScanReport, err error) {
	if err := ctx.Err(); err != nil {
		return report, err
	}
	fields, err := selection.compile()
	if err != nil {
		return report, err
	}
	if !strings.EqualFold(path.Ext(asset.Name), ".csv") && !strings.EqualFold(asset.Format, "CSV") {
		return report, fmt.Errorf("full CSV scan requires an inspected direct CSV asset")
	}
	res, err := i.openAsset(ctx, asset.Request)
	if err != nil {
		return report, err
	}
	defer res.Body.Close()
	return scanCSVResponse(ctx, res, fields, visit)
}

// Static assets and inspected exports share the complete parser and byte hash.
// The caller owns and closes Body; interim visitor state is discarded on error.
func scanCSVResponse(ctx context.Context, res *fetch.StreamResponse, fields map[string]map[string]bool, visit func(CSVScanRecord) error) (report CSVScanReport, err error) {
	if res.Status != http.StatusOK {
		return report, fmt.Errorf("CSV scan download status %d", res.Status)
	}
	if res.ContentLength > maxCSVScanBytes {
		return report, fmt.Errorf("CSV full scan exceeds 64 MiB")
	}
	reader := &csvScanByteReader{r: sampleContextReader{ctx: ctx, r: res.Body}, hash: sha256.New()}
	defer func() { report.Bytes = reader.total }()
	br := bufio.NewReader(reader)
	if prefix, _ := br.Peek(3); len(prefix) == 3 && prefix[0] == 0xef && prefix[1] == 0xbb && prefix[2] == 0xbf {
		_, _ = br.Discard(3)
	}
	headerBytes, readErr := br.ReadBytes('\n')
	if readErr != nil && readErr != io.EOF {
		return report, fmt.Errorf("CSV header read: %w", readErr)
	}
	var decoded io.Reader = io.MultiReader(bytes.NewReader(headerBytes), br)
	report.Encoding = "utf-8"
	if !utf8.Valid(headerBytes) {
		report.Encoding = "euc-kr"
		decoded = transform.NewReader(decoded, newStrictEUCKRDecoder())
	}
	r := csv.NewReader(decoded)
	header, err := r.Read()
	if err != nil {
		return report, fmt.Errorf("CSV scan header: %w", err)
	}
	if len(header) == 0 || len(header) > 256 {
		return report, fmt.Errorf("CSV scan needs 1–256 unique columns")
	}
	seen := map[string]bool{}
	for n, h := range header {
		if !utf8.ValidString(h) {
			return report, fmt.Errorf("CSV scan header is invalid under the selected %s decoding", report.Encoding)
		}
		h = strings.TrimSpace(h)
		if h == "" || len(h) > 256 || seen[h] {
			return report, fmt.Errorf("CSV scan needs unique nonblank headers of at most 256 bytes")
		}
		seen[h] = true
		header[n] = h
	}
	for field := range fields {
		if !seen[field] {
			return report, fmt.Errorf("CSV scan selection field %q not in observed header", field)
		}
	}
	report.Columns = append([]string(nil), header...)
	for {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		reader.window = 0
		values, err := r.Read()
		if err == io.EOF {
			if err := ctx.Err(); err != nil {
				return report, err
			}
			report.Exhausted = true
			report.SHA256 = fmt.Sprintf("%x", reader.hash.Sum(nil))
			return report, nil
		}
		if err != nil {
			return report, fmt.Errorf("CSV full scan record: %w", err)
		}
		report.ScannedRows++
		if report.ScannedRows > 2_000_000 {
			return report, fmt.Errorf("CSV scan exceeds 2 million data records")
		}
		rowBytes := 0
		matches := true
		for n, v := range values {
			if !utf8.ValidString(v) {
				return report, fmt.Errorf("CSV scan contains invalid UTF-8 at data record %d", report.ScannedRows)
			}
			rowBytes += len(v)
			if len(v) > 64<<10 || rowBytes > 1<<20 {
				return report, fmt.Errorf("CSV scan field/record size limit exceeded")
			}
			if allowed, required := fields[header[n]]; required && !allowed[v] {
				matches = false
			}
		}
		if !matches {
			continue
		}
		report.MatchedRows++
		if visit != nil {
			row := make(map[string]string, len(header))
			for n, h := range header {
				row[h] = values[n]
			}
			line, _ := r.FieldPos(0)
			if err := visit(CSVScanRecord{DataRecord: report.ScannedRows, StartLine: line, Values: row}); err != nil {
				return report, err
			}
		}
	}
}

// SampleCSVScanned retains a bounded prefix but continues scanning all matching
// and excluded rows. It deliberately separates full scan from retained coverage.
func (i *Inspector) SampleCSVScanned(ctx context.Context, asset Asset, limit int, selection CSVSelection) (TableSample, error) {
	if limit < 1 || limit > 1000 {
		return TableSample{}, fmt.Errorf("sample row limit must be 1–1000")
	}
	out := TableSample{CSV: &CSVProvenance{Encoding: "utf-8"}}
	retainedBytes := 2
	report, err := i.ScanCSV(ctx, asset, selection, func(record CSVScanRecord) error {
		if len(out.Rows) == limit {
			return nil
		}
		row := map[string]any{}
		for k, v := range record.Values {
			row[k] = v
		}
		encoded, err := json.Marshal(row)
		if err != nil {
			return err
		}
		retainedBytes += len(encoded) + 1
		if retainedBytes > 2<<20 {
			return fmt.Errorf("CSV retained sample exceeds 2 MiB; narrow selection")
		}
		out.Rows = append(out.Rows, row)
		out.CSV.DataRecords = append(out.CSV.DataRecords, record.DataRecord)
		out.CSV.StartLines = append(out.CSV.StartLines, record.StartLine)
		return nil
	})
	if err != nil {
		return TableSample{}, err
	}
	if len(out.Rows) == 0 {
		return TableSample{}, fmt.Errorf("no CSV rows matched after scanning %d asset rows; not population absence evidence", report.ScannedRows)
	}
	out.SHA256, out.Bytes = report.SHA256, report.Bytes
	out.CSV.Encoding = report.Encoding
	out.Prefix = report.MatchedRows > len(out.Rows)
	out.Selection = &SelectionReport{Mode: "exact_strings_full_scan_v1", ScannedRows: report.ScannedRows, MatchedRows: report.MatchedRows, ReturnedRows: len(out.Rows), Exhausted: report.Exhausted}
	if len(selection.In) != 0 {
		out.Selection.Mode = "exact_string_sets_full_scan_v1"
	}
	out.Warnings = []string{"Direct comma-delimited CSV fully scanned (64 MiB cap); all parsed rows checked, including excluded rows and the tail. The first physical header line selects UTF-8 or lossless EUC-KR, with no midstream fallback. Only the first matching rows are retained. Exhausted describes scanning, NOT retention of all matches or population completeness. Source bytes were hashed as a stream, not archived. No identity or geographic approval."}
	return out, nil
}

// Bounds both total transport bytes and bytes read during one csv.Read call.
// The latter bounds parser allocation even for an unterminated giant field;
// the parser's small read-ahead buffer does not accumulate the whole source.
type csvScanByteReader struct {
	r      io.Reader
	hash   hash.Hash
	total  int64
	window int
}

func (r *csvScanByteReader) Read(p []byte) (int, error) {
	remaining := int64(maxCSVScanBytes) + 1 - r.total
	windowRemaining := int64(2<<20) + 1 - int64(r.window)
	if remaining <= 0 {
		return 0, fmt.Errorf("CSV full scan exceeds 64 MiB")
	}
	if windowRemaining <= 0 {
		return 0, fmt.Errorf("CSV record parsing byte limit exceeded")
	}
	if remaining > windowRemaining {
		remaining = windowRemaining
	}
	if int64(len(p)) > remaining {
		p = p[:int(remaining)]
	}
	n, err := r.r.Read(p)
	r.total += int64(n)
	r.window += n
	if n > 0 {
		_, _ = r.hash.Write(p[:n])
	}
	if r.total > maxCSVScanBytes {
		return n, fmt.Errorf("CSV full scan exceeds 64 MiB")
	}
	if r.window > 2<<20 {
		return n, fmt.Errorf("CSV record parsing byte limit exceeded")
	}
	return n, err
}

// EUC-KR/CP949 has no shift state. Decode one code unit and round-trip it before
// releasing UTF-8 bytes, so malformed input cannot become replacement text.
type strictEUCKRDecoder struct {
	decoder transform.Transformer
	encoder transform.Transformer
}

func newStrictEUCKRDecoder() *strictEUCKRDecoder {
	return &strictEUCKRDecoder{decoder: korean.EUCKR.NewDecoder(), encoder: korean.EUCKR.NewEncoder()}
}
func (d *strictEUCKRDecoder) Reset() { d.decoder.Reset(); d.encoder.Reset() }
func (d *strictEUCKRDecoder) Transform(dst, src []byte, atEOF bool) (nd, ns int, err error) {
	var decoded, encoded [8]byte
	for ns < len(src) {
		if src[ns] < 0x80 {
			if nd == len(dst) {
				return nd, ns, transform.ErrShortDst
			}
			dst[nd] = src[ns]
			nd++
			ns++
			continue
		}
		if len(src)-ns < 2 {
			if !atEOF {
				return nd, ns, transform.ErrShortSrc
			}
			return nd, ns, fmt.Errorf("incomplete EUC-KR CSV code unit")
		}
		unit := src[ns : ns+2]
		n, outConsumed, e := d.decoder.Transform(decoded[:], unit, true)
		if e != nil || outConsumed != 2 {
			return nd, ns, fmt.Errorf("invalid EUC-KR CSV code unit")
		}
		m, inConsumed, e := d.encoder.Transform(encoded[:], decoded[:n], true)
		if e != nil || inConsumed != n || m != 2 || !bytes.Equal(encoded[:m], unit) {
			return nd, ns, fmt.Errorf("lossy EUC-KR CSV decoding rejected")
		}
		if len(dst)-nd < n {
			return nd, ns, transform.ErrShortDst
		}
		copy(dst[nd:], decoded[:n])
		nd += n
		ns += 2
	}
	return nd, ns, nil
}
