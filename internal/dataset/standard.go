package dataset

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"github.com/JungHoonGhae/odeduck/internal/portal"
)

// StandardContract describes native STD, not a credentialled API or invented
// CSV asset. Only the private handle can supply the server-returned selectors.
type StandardContract struct {
	PK              string           `json:"pk"`
	Name            string           `json:"name"`
	SourceURL       string           `json:"sourceUrl"`
	Columns         []StandardColumn `json:"columns"`
	TotalCount      int64            `json:"totalCount"` // publisher claim, not observed population
	SchemaSHA256    string           `json:"schemaSha256"`
	ObservedAt      string           `json:"observedAt"`
	AdapterID       string           `json:"adapterId"`
	AdapterRevision int              `json:"adapterRevision"`
	Evidence        []Evidence       `json:"evidence"`
	Warnings        []string         `json:"warnings"`
	handle          *standardHandle
}

type StandardColumn struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type standardHandle struct {
	owner   *Inspector
	pk      string
	query   url.Values
	columns map[string]bool
	total   int64
}

var standardIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

func (i *Inspector) inspectStandard(ctx context.Context, pk string) (*StandardContract, error) {
	if err := portal.ValidatePublicDataPK(pk); err != nil {
		return nil, err
	}
	endpoint := i.base + "/download/columList.json?" + url.Values{"pk": {pk}, "ext": {"CSV"}}.Encode()
	body, err := i.standardBytes(ctx, endpoint, 512<<10)
	if err != nil {
		return nil, err
	}
	if err := validateStandardJSONText(body); err != nil {
		return nil, err
	}
	var header struct {
		Name    string `json:"fileName"`
		Columns []struct {
			Code string `json:"columCode"`
			Name string `json:"columNm"`
		} `json:"columList"`
		Table struct {
			PK      string   `json:"publicDataPk"`
			Name    string   `json:"svcTableNm"`
			Columns []string `json:"colNmList"`
		} `json:"tableVO"`
		Total *int64 `json:"totalCount"`
	}
	if err := json.Unmarshal(body, &header); err != nil {
		return nil, fmt.Errorf("standard header is not valid JSON: %w", err)
	}
	if header.Table.PK != pk || strings.TrimSpace(header.Name) == "" || len(header.Name) > 1000 || !standardIdentifier.MatchString(header.Table.Name) || header.Total == nil || *header.Total < 0 || len(header.Columns) == 0 || len(header.Columns) > 256 || len(header.Table.Columns) == 0 || len(header.Table.Columns) > 256 {
		return nil, fmt.Errorf("standard header has no proven PK/schema/table/count contract")
	}
	columns := map[string]bool{}
	contract := &StandardContract{PK: pk, Name: header.Name, SourceURL: i.base + "/data/" + pk + "/standard.do", TotalCount: *header.Total, SchemaSHA256: fmt.Sprintf("%x", sha256.Sum256(body)), ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), AdapterID: "datagokr-standard", AdapterRevision: 1,
		Evidence: []Evidence{{Purpose: "schema-and-retrieval", Kind: EvidenceFirstPartyWebContract, URL: endpoint, Stability: StabilityFallback}},
		Warnings: []string{"First-party web contract, not a documented OpenAPI. Declared totalCount is not observed population coverage.", "Rows can predate the catalogue modification date; inspect each record's reference/operation dates before temporal comparisons."}}
	for _, c := range header.Columns {
		if !standardIdentifier.MatchString(c.Code) || columns[c.Code] || strings.TrimSpace(c.Name) == "" || len(c.Name) > 1000 {
			return nil, fmt.Errorf("standard header has invalid or duplicate column")
		}
		columns[c.Code] = true
		contract.Columns = append(contract.Columns, StandardColumn{Code: c.Code, Name: c.Name})
	}
	selected := map[string]bool{}
	for _, c := range header.Table.Columns {
		if !columns[c] || selected[c] {
			return nil, fmt.Errorf("standard selectors are not unique advertised columns")
		}
		selected[c] = true
	}
	contract.handle = &standardHandle{owner: i, pk: pk, total: *header.Total, columns: columns, query: url.Values{"publicDataPk": {pk}, "svcTableNm": {header.Table.Name}, "totalCount": {strconv.FormatInt(*header.Total, 10)}, "colNmList": append([]string(nil), header.Table.Columns...)}}
	return contract, nil
}

// SampleStandard uses a contract inspected by this exact instance. It reads a
// bounded first page, not a bulk download or population pagination operation.
func (i *UnifiedInspector) SampleStandard(ctx context.Context, contract *StandardContract, limit int) (TableSample, error) {
	if i == nil || i.files == nil {
		return TableSample{}, fmt.Errorf("standard inspector is not initialized")
	}
	return i.files.sampleStandard(ctx, contract, limit)
}

func (i *Inspector) sampleStandard(ctx context.Context, contract *StandardContract, limit int) (TableSample, error) {
	if contract == nil || contract.handle == nil || contract.handle.owner != i || contract.PK != contract.handle.pk {
		return TableSample{}, fmt.Errorf("missing trusted standard inspection handle")
	}
	if limit < 1 || limit > 1000 {
		return TableSample{}, fmt.Errorf("standard sample row limit must be 1–1000")
	}
	h := contract.handle
	q := url.Values{}
	for k, v := range h.query {
		q[k] = append([]string(nil), v...)
	}
	q.Set("perPage", strconv.Itoa(limit))
	q.Set("page", "1")
	body, err := i.standardBytes(ctx, i.base+"/download/standard.json?"+q.Encode(), 2<<20)
	if err != nil {
		return TableSample{}, err
	}
	if err := validateStandardJSONText(body); err != nil {
		return TableSample{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var rows []map[string]any
	if err = decoder.Decode(&rows); err != nil {
		return TableSample{}, fmt.Errorf("standard rows: %w", err)
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return TableSample{}, fmt.Errorf("standard response contains trailing JSON")
	}
	if len(rows) == 0 || len(rows) > limit {
		return TableSample{}, fmt.Errorf("standard response must contain 1–%d rows", limit)
	}
	for _, row := range rows {
		if len(row) == 0 {
			return TableSample{}, fmt.Errorf("standard response contains empty record")
		}
		for key, value := range row {
			if !h.columns[key] {
				return TableSample{}, fmt.Errorf("standard response contains unadvertised column %q", key)
			}
			switch value.(type) {
			case nil, string, bool, json.Number:
			default:
				return TableSample{}, fmt.Errorf("standard column %q is not scalar", key)
			}
		}
	}
	return TableSample{Rows: rows, SHA256: fmt.Sprintf("%x", sha256.Sum256(body)), Bytes: int64(len(body)), Prefix: h.total > int64(len(rows)) || len(rows) == limit}, nil
}

// encoding/json replaces invalid UTF-8 and lone UTF-16 surrogates with U+FFFD.
// Reject that lossy conversion before decoding either schema or source rows:
// different malformed identifiers must never become equal retained strings.
func validateStandardJSONText(body []byte) error {
	if !utf8.Valid(body) || !json.Valid(body) {
		return fmt.Errorf("standard response requires valid UTF-8 JSON")
	}
	// JSON syntax is already validated, so every backslash is a complete string
	// escape. Skip escaped backslashes too: literal \\ud800 is not a surrogate.
	for n := 0; n < len(body); n++ {
		if body[n] != '\\' {
			continue
		}
		if body[n+1] != 'u' {
			n++
			continue
		}
		code, _ := strconv.ParseUint(string(body[n+2:n+6]), 16, 16)
		switch {
		case code >= 0xD800 && code <= 0xDBFF:
			if n+12 > len(body) || body[n+6] != '\\' || body[n+7] != 'u' {
				return fmt.Errorf("standard response contains an unpaired Unicode surrogate")
			}
			low, err := strconv.ParseUint(string(body[n+8:n+12]), 16, 16)
			if err != nil || low < 0xDC00 || low > 0xDFFF {
				return fmt.Errorf("standard response contains an unpaired Unicode surrogate")
			}
			n += 6
		case code >= 0xDC00 && code <= 0xDFFF:
			return fmt.Errorf("standard response contains an unpaired Unicode surrogate")
		}
		n += 5
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	return validateStandardJSONMembers(decoder, 0)
}

// Validate before struct/map decoding can overwrite duplicate object members.
// Escaped spellings are compared after decoding; separate records keep their
// own member sets. The bounded walk also caps unexpected nested metadata.
func validateStandardJSONMembers(decoder *json.Decoder, depth int) error {
	if depth > 32 {
		return fmt.Errorf("standard JSON exceeds 32 nesting levels")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	switch token {
	case json.Delim('{'):
		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fmt.Errorf("standard JSON contains duplicate object members")
			}
			seen[name] = true
			if err := validateStandardJSONMembers(decoder, depth+1); err != nil {
				return err
			}
		}
	case json.Delim('['):
		for decoder.More() {
			if err := validateStandardJSONMembers(decoder, depth+1); err != nil {
				return err
			}
		}
	default:
		return nil
	}
	_, err = decoder.Token() // json.Valid already established matching delimiters.
	return err
}

func (i *Inspector) standardBytes(ctx context.Context, endpoint string, limit int64) ([]byte, error) {
	// STD evidence is first-party data, unlike redirectable FILE downloads.
	// Require the bounded no-redirect transport in production and fixtures alike.
	stream, ok := i.http.(interface {
		OpenGETNoRedirect(context.Context, string) (*fetch.StreamResponse, error)
	})
	if !ok {
		return nil, fmt.Errorf("standard inspection requires a no-redirect streaming transport")
	}
	res, err := stream.OpenGETNoRedirect(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.Status != http.StatusOK {
		return nil, fmt.Errorf("standard download status %d", res.Status)
	}
	if res.ContentLength > limit {
		return nil, fmt.Errorf("standard response exceeds %d bytes", limit)
	}
	body, err := io.ReadAll(io.LimitReader(sampleContextReader{ctx: ctx, r: res.Body}, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("standard response exceeds %d bytes", limit)
	}
	return body, ctx.Err()
}
