package dataset

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// XLSXSelection addresses original cells, never inferred or merged headers.
type XLSXSelection struct {
	Sheet string `json:"sheet"`
	Range string `json:"range"`
}

// TableProvenance identifies the exact source rectangle. Full rectangle reading
// says nothing about whether those cells cover the user's target population.
// FormulaCells contain cached observations, never recalculated results.
type TableProvenance struct {
	Sheet                string   `json:"sheet"`
	Range                string   `json:"range"`
	Member               string   `json:"member"`
	RowNumbers           []int    `json:"rowNumbers"`
	FormulaCells         []string `json:"formulaCells,omitempty"`
	FormulasWithoutCache int      `json:"formulasWithoutCache"`
}

var exactXLSXCoordinate = regexp.MustCompile(`^[A-Z]{1,3}[1-9][0-9]{0,6}$`)

func strictXLSXCoordinate(ref string) (xlsxCoordinate, bool) {
	if !exactXLSXCoordinate.MatchString(ref) {
		return xlsxCoordinate{}, false
	}
	c, ok := parseXLSXCoordinate(ref)
	return c, ok && c.Column < 16384 && c.Row <= 1048576
}

func xlsxSelectionBounds(s XLSXSelection) (xlsxCoordinate, xlsxCoordinate, error) {
	var zero xlsxCoordinate
	parts := strings.Split(s.Range, ":")
	if len(parts) == 1 {
		parts = append(parts, parts[0])
	}
	if strings.TrimSpace(s.Sheet) == "" || len(s.Sheet) > 256 || len(parts) != 2 {
		return zero, zero, fmt.Errorf("XLSX requires an exact sheet name and uppercase A1 rectangle")
	}
	a, aOK := strictXLSXCoordinate(parts[0])
	b, bOK := strictXLSXCoordinate(parts[1])
	if !aOK || !bOK || b.Row < a.Row || b.Column < a.Column || b.Row-a.Row >= 1000 || b.Column-a.Column >= maxXLSXColumns {
		return zero, zero, fmt.Errorf("XLSX rectangle must be ordered uppercase A1 coordinates, at most 1000 rows and 256 columns")
	}
	return a, b, nil
}

func ValidateXLSXSelection(s XLSXSelection) error {
	_, _, err := xlsxSelectionBounds(s)
	return err
}

// SampleXLSX reads a bounded exact rectangle from an inspected direct asset.
// It preserves text whitespace/leading zeroes and numeric storage lexemes.
// It does not interpret number formats, dates, merged cells or formula code.
func (i *Inspector) SampleXLSX(ctx context.Context, asset Asset, selection XLSXSelection) (TableSample, error) {
	if err := ctx.Err(); err != nil {
		return TableSample{}, err
	}
	lo, hi, err := xlsxSelectionBounds(selection)
	if err != nil {
		return TableSample{}, err
	}
	body, err := i.downloadSampleAsset(ctx, asset, "XLSX")
	if err != nil {
		return TableSample{}, err
	}
	members, err := strictXLSXMembers(body)
	if err != nil {
		return TableSample{}, err
	}
	member, err := exactXLSXSheet(members, selection.Sheet)
	if err != nil {
		return TableSample{}, err
	}
	rows, err := readXLSXRectangle(ctx, member, lo, hi)
	if err != nil {
		return TableSample{}, err
	}
	wanted := map[int]struct{}{}
	for _, row := range rows {
		for _, cell := range row.Cells {
			if cell.Type != "s" {
				continue
			}
			index, err := strconv.Atoi(cell.Value)
			if err != nil || index < 0 || index >= maxXLSXSharedStringItems {
				return TableSample{}, fmt.Errorf("invalid shared string reference at %s", cell.Ref)
			}
			wanted[index] = struct{}{}
		}
	}
	shared, err := decodeXLSXSharedStringsMode(members["xl/sharedStrings.xml"], wanted, false)
	if err != nil {
		return TableSample{}, err
	}
	result := TableSample{SHA256: fmt.Sprintf("%x", sha256.Sum256(body)), Bytes: int64(len(body)),
		Table:    &TableProvenance{Sheet: selection.Sheet, Range: selection.Range, Member: member.Name},
		Warnings: []string{"Exact XLSX rectangle only; original column letters and row numbers are retained. Blank/absent cells are null. Headers/merged cells are not inferred or filled. Numeric storage values remain strings; styles and date serials are not interpreted. Formula caches are not recalculated or freshness-verified. This is not population completeness evidence."}}
	byRow := map[int]xlsxRow{}
	for _, row := range rows {
		byRow[row.Number] = row
	}
	retained := 0
	for number := lo.Row; number <= hi.Row; number++ {
		if err := ctx.Err(); err != nil {
			return TableSample{}, err
		}
		out := map[string]any{}
		for col := lo.Column; col <= hi.Column; col++ {
			out[xlsxColumnName(col)] = nil
		}
		for _, cell := range byRow[number].Cells {
			value, err := exactXLSXCellValue(cell, shared)
			if err != nil {
				return TableSample{}, err
			}
			if s, ok := value.(string); ok {
				retained += len(s)
			}
			if retained > 2<<20 {
				return TableSample{}, fmt.Errorf("XLSX selected cell values exceed 2 MiB")
			}
			coordinate, _ := strictXLSXCoordinate(cell.Ref)
			out[xlsxColumnName(coordinate.Column)] = value
			if cell.Formula {
				if len(result.Table.FormulaCells) >= 4096 {
					return TableSample{}, fmt.Errorf("XLSX formula provenance exceeds 4096 cells")
				}
				result.Table.FormulaCells = append(result.Table.FormulaCells, cell.Ref)
				if !cell.HasValue || cell.Value == "" {
					result.Table.FormulasWithoutCache++
				}
			}
		}
		result.Rows = append(result.Rows, out)
		result.Table.RowNumbers = append(result.Table.RowNumbers, number)
	}
	return result, nil
}

func strictXLSXMembers(body []byte) (map[string]*zip.File, error) {
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, err
	}
	if len(zr.File) > maxArchiveEntries {
		return nil, fmt.Errorf("XLSX archive exceeds entry limit")
	}
	members := map[string]*zip.File{}
	var expanded uint64
	for _, m := range zr.File {
		if m.UncompressedSize64 > maxExpandedBytes-expanded {
			return nil, fmt.Errorf("XLSX expanded size exceeds 64 MiB")
		}
		expanded += m.UncompressedSize64
		name := strings.TrimSuffix(m.Name, "/")
		if name == "." || path.Clean(name) != name || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "../") || strings.ContainsAny(name, `\:`) || members[name] != nil {
			return nil, fmt.Errorf("ambiguous or unsafe XLSX member name")
		}
		members[name] = m
	}
	return members, nil
}

func exactXLSXSheet(members map[string]*zip.File, name string) (*zip.File, error) {
	book, err := decodeXLSXWorkbook(members["xl/workbook.xml"])
	if err != nil {
		return nil, err
	}
	rels, err := decodeXLSXRelationships(members["xl/_rels/workbook.xml.rels"])
	if err != nil {
		return nil, err
	}
	names, sheetIDs := map[string]bool{}, map[string]bool{}
	id := ""
	for _, s := range book.Sheets {
		if s.Name == "" || s.RelationshipID == "" || names[s.Name] || sheetIDs[s.RelationshipID] {
			return nil, fmt.Errorf("ambiguous XLSX sheet identity")
		}
		names[s.Name], sheetIDs[s.RelationshipID] = true, true
		if s.Name == name {
			id = s.RelationshipID
		}
	}
	if id == "" {
		return nil, fmt.Errorf("exact XLSX sheet not found")
	}
	seen := map[string]bool{}
	target := ""
	for _, r := range rels.Items {
		if r.ID == "" || seen[r.ID] {
			return nil, fmt.Errorf("ambiguous XLSX relationship ID")
		}
		seen[r.ID] = true
		if r.ID != id {
			continue
		}
		if r.TargetMode != "" && r.TargetMode != "Internal" {
			return nil, fmt.Errorf("external XLSX sheet is forbidden")
		}
		if r.Type != "http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" && r.Type != "http://purl.oclc.org/ooxml/officeDocument/relationships/worksheet" {
			return nil, fmt.Errorf("unsupported XLSX sheet relationship")
		}
		target = r.Target
		if strings.ContainsAny(target, `\:?#%`) || strings.Contains(target, "..") {
			return nil, fmt.Errorf("unsafe XLSX sheet target")
		}
		if strings.HasPrefix(target, "/") {
			target = strings.TrimPrefix(target, "/")
		} else {
			target = "xl/" + target
		}
		if path.Clean(target) != target || !strings.HasPrefix(target, "xl/worksheets/") {
			return nil, fmt.Errorf("XLSX sheet target is outside worksheets")
		}
	}
	if target == "" || members[target] == nil {
		return nil, fmt.Errorf("XLSX sheet relationship target missing")
	}
	return members[target], nil
}

// Scan the complete XML, including the tail, while retaining only selected cells.
// Coordinates must be explicit and ordered; unsupported implicit positions fail.
func readXLSXRectangle(ctx context.Context, member *zip.File, lo, hi xlsxCoordinate) ([]xlsxRow, error) {
	var rows []xlsxRow
	err := streamXLSXMember(member, func(d *xml.Decoder) error {
		rootSeen, rootClosed, dataSeen, inData := false, false, false, false
		lastRow, retained := 0, 0
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			t, err := d.Token()
			if err == io.EOF {
				if !rootClosed || !dataSeen {
					return fmt.Errorf("incomplete XLSX worksheet")
				}
				return nil
			}
			if err != nil {
				return err
			}
			switch t := t.(type) {
			case xml.StartElement:
				if !rootSeen {
					if t.Name.Local != "worksheet" {
						return fmt.Errorf("invalid XLSX worksheet root")
					}
					rootSeen = true
					continue
				}
				if rootClosed {
					return fmt.Errorf("multiple XLSX worksheet roots")
				}
				if t.Name.Local == "sheetData" && !inData {
					if dataSeen {
						return fmt.Errorf("duplicate XLSX sheetData")
					}
					dataSeen, inData = true, true
					continue
				}
				if !inData {
					if err := d.Skip(); err != nil {
						return err
					}
					continue
				}
				if t.Name.Local != "row" {
					return fmt.Errorf("unexpected XLSX sheetData child")
				}
				n, err := strconv.Atoi(xlsxAttribute(t.Attr, "r"))
				if err != nil || n <= lastRow || n > 1048576 {
					return fmt.Errorf("invalid or duplicate XLSX row position")
				}
				lastRow = n
				row, err := readExactXLSXRow(ctx, d, t, n, lo, hi, &retained)
				if err != nil {
					return err
				}
				if n >= lo.Row && n <= hi.Row {
					rows = append(rows, row)
				}
			case xml.EndElement:
				if t.Name.Local == "sheetData" {
					inData = false
				}
				if t.Name.Local == "worksheet" {
					rootClosed = true
				}
			case xml.CharData:
				if strings.TrimSpace(string(t)) != "" {
					return fmt.Errorf("unexpected XLSX worksheet text")
				}
			}
		}
	})
	return rows, err
}

func readExactXLSXRow(ctx context.Context, d *xml.Decoder, start xml.StartElement, number int, lo, hi xlsxCoordinate, retained *int) (xlsxRow, error) {
	row := xlsxRow{Number: number}
	lastColumn := -1
	for {
		if err := ctx.Err(); err != nil {
			return row, err
		}
		t, err := d.Token()
		if err != nil {
			return row, err
		}
		switch t := t.(type) {
		case xml.StartElement:
			if t.Name.Local != "c" {
				return row, fmt.Errorf("unexpected XLSX row child")
			}
			c, ok := strictXLSXCoordinate(xlsxAttribute(t.Attr, "r"))
			if !ok || c.Row != number || c.Column <= lastColumn {
				return row, fmt.Errorf("invalid or duplicate XLSX cell position")
			}
			lastColumn = c.Column
			if number < lo.Row || number > hi.Row || c.Column < lo.Column || c.Column > hi.Column {
				if err := d.Skip(); err != nil {
					return row, err
				}
				continue
			}
			cell, err := decodeXLSXCell(d, t)
			if err != nil {
				return row, err
			}
			*retained += len(cell.Value) + len(cell.Inline.Text)
			if *retained > 2<<20 {
				return row, fmt.Errorf("XLSX selected raw values exceed 2 MiB")
			}
			row.Cells = append(row.Cells, cell)
		case xml.EndElement:
			if t.Name == start.Name {
				return row, nil
			}
		case xml.CharData:
			if strings.TrimSpace(string(t)) != "" {
				return row, fmt.Errorf("unexpected XLSX row text")
			}
		}
	}
}

func exactXLSXCellValue(c xlsxCell, shared map[int]string) (any, error) {
	switch c.Type {
	case "", "n", "s", "inlineStr", "b", "str", "d", "e":
	default:
		return nil, fmt.Errorf("unsupported XLSX cell storage type at %s", c.Ref)
	}
	if c.Type == "e" {
		return nil, fmt.Errorf("XLSX error cell at %s", c.Ref)
	}
	if c.Formula && (!c.HasValue || c.Value == "") {
		return nil, nil
	}
	switch c.Type {
	case "s":
		n, err := strconv.Atoi(c.Value)
		value, exists := shared[n]
		if err != nil || !exists {
			return nil, fmt.Errorf("missing XLSX shared string at %s", c.Ref)
		}
		return value, nil
	case "inlineStr":
		return c.Inline.Text, nil
	case "b":
		if c.Value == "0" {
			return false, nil
		}
		if c.Value == "1" {
			return true, nil
		}
		return nil, fmt.Errorf("invalid XLSX boolean at %s", c.Ref)
	case "", "n", "str", "d":
		if !c.HasValue || c.Value == "" {
			return nil, nil
		}
		return c.Value, nil
	default:
		return nil, fmt.Errorf("unsupported XLSX cell storage type at %s", c.Ref)
	}
}
