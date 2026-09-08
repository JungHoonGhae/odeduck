package dataset

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// FileLayout contains only asset metadata and physical structure. It never
// exports cell contents, inferred headers, formula code or calculated values.
type FileLayout struct {
	Format   string           `json:"format"`
	Members  []ZIPMember      `json:"members,omitempty"`
	SHA256   string           `json:"sha256"`
	Bytes    int64            `json:"bytes"`
	Sheets   []XLSXSheetRef   `json:"sheets"`
	Sheet    *XLSXSheetLayout `json:"sheet,omitempty"`
	Warnings []string         `json:"warnings"`
}
type XLSXSheetRef struct {
	Name   string `json:"name"`
	Member string `json:"member"`
}
type XLSXSheetLayout struct {
	Name            string         `json:"name"`
	Member          string         `json:"member"`
	Extent          string         `json:"extent,omitempty"`
	StoredRows      int            `json:"storedRows"`
	StoredCells     int            `json:"storedCells"`
	Rows            []XLSXRowShape `json:"rows,omitempty"`
	RowsTruncated   bool           `json:"rowsTruncated"`
	Merges          []string       `json:"merges,omitempty"`
	MergesTruncated bool           `json:"mergesTruncated"`
}
type XLSXRowShape struct {
	Number int    `json:"number"`
	First  string `json:"first,omitempty"`
	Last   string `json:"last,omitempty"`
	Cells  int    `json:"cells"`
}

// LayoutXLSX first lists exact worksheet identities. Selecting one sheet scans
// its complete XML for positions, retaining only the first 128 row shapes and
// 128 merge ranges. Extent is based on stored cells, not the optional/stale
// dimension attribute. Structural coverage does not establish data coverage.
func (i *Inspector) LayoutXLSX(ctx context.Context, asset Asset, sheet string) (FileLayout, error) {
	if len(sheet) > 256 {
		return FileLayout{}, fmt.Errorf("XLSX sheet name exceeds 256 bytes")
	}
	body, err := i.downloadSampleAsset(ctx, asset, "XLSX")
	if err != nil {
		return FileLayout{}, err
	}
	members, err := strictXLSXMembers(body)
	if err != nil {
		return FileLayout{}, err
	}
	book, err := decodeXLSXWorkbook(members["xl/workbook.xml"])
	if err != nil {
		return FileLayout{}, err
	}
	out := FileLayout{Format: "XLSX", SHA256: fmt.Sprintf("%x", sha256.Sum256(body)), Bytes: int64(len(body)), Warnings: []string{"Physical XLSX structure only. Stored cells may be blank/style-only; extent and row counts do not identify tables, headers, population coverage or valid geography/time. Cell text and formula code are not returned. Worksheet names are untrusted publisher metadata."}}
	var selected *zip.File
	for _, s := range book.Sheets {
		if len(s.Name) > 256 {
			return FileLayout{}, fmt.Errorf("XLSX sheet name exceeds metadata limit")
		}
		member, err := exactXLSXSheet(members, s.Name)
		if err != nil {
			return FileLayout{}, err
		}
		if len(member.Name) > 512 {
			return FileLayout{}, fmt.Errorf("XLSX member name exceeds metadata limit")
		}
		out.Sheets = append(out.Sheets, XLSXSheetRef{Name: s.Name, Member: member.Name})
		if s.Name == sheet {
			selected = member
		}
	}
	if len(out.Sheets) == 0 {
		return FileLayout{}, fmt.Errorf("XLSX workbook has no worksheets")
	}
	if sheet == "" {
		return out, nil
	}
	if selected == nil {
		return FileLayout{}, fmt.Errorf("exact XLSX sheet not found")
	}
	l, err := scanXLSXLayout(ctx, selected)
	if err != nil {
		return FileLayout{}, err
	}
	l.Name, l.Member = sheet, selected.Name
	out.Sheet = &l
	return out, nil
}

func scanXLSXLayout(ctx context.Context, member *zip.File) (XLSXSheetLayout, error) {
	l := XLSXSheetLayout{}
	minCol, minRow, maxCol, maxRow := 16384, 1048577, -1, 0
	err := streamXLSXMember(member, func(d *xml.Decoder) error {
		rootSeen, closed, dataSeen, inData, inMerges, mergesSeen := false, false, false, false, false, false
		lastRow := 0
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			t, err := d.Token()
			if err == io.EOF {
				if !closed || !dataSeen {
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
				if closed {
					return fmt.Errorf("multiple XLSX worksheet roots")
				}
				if inData {
					if t.Name.Local != "row" {
						return fmt.Errorf("unexpected XLSX sheetData child")
					}
					n, err := strconv.Atoi(xlsxAttribute(t.Attr, "r"))
					if err != nil || n <= lastRow || n > 1048576 {
						return fmt.Errorf("invalid or duplicate XLSX row position")
					}
					lastRow = n
					shape, first, last, err := scanXLSXRowShape(ctx, d, t, n)
					if err != nil {
						return err
					}
					l.StoredRows++
					l.StoredCells += shape.Cells
					if len(l.Rows) < 128 {
						l.Rows = append(l.Rows, shape)
					} else {
						l.RowsTruncated = true
					}
					if shape.Cells > 0 {
						if first < minCol {
							minCol = first
						}
						if last > maxCol {
							maxCol = last
						}
						if n < minRow {
							minRow = n
						}
						if n > maxRow {
							maxRow = n
						}
					}
					continue
				}
				if inMerges {
					if t.Name.Local != "mergeCell" {
						return fmt.Errorf("unexpected XLSX merge child")
					}
					ref := xlsxAttribute(t.Attr, "ref")
					parts := strings.Split(ref, ":")
					if len(parts) != 2 {
						return fmt.Errorf("invalid XLSX merge range")
					}
					a, aOK := strictXLSXCoordinate(parts[0])
					b, bOK := strictXLSXCoordinate(parts[1])
					if !aOK || !bOK || b.Row < a.Row || b.Column < a.Column {
						return fmt.Errorf("invalid XLSX merge range")
					}
					if len(l.Merges) < 128 {
						l.Merges = append(l.Merges, ref)
					} else {
						l.MergesTruncated = true
					}
					if err := d.Skip(); err != nil {
						return err
					}
					continue
				}
				switch t.Name.Local {
				case "sheetData":
					if dataSeen {
						return fmt.Errorf("duplicate XLSX sheetData")
					}
					dataSeen, inData = true, true
				case "mergeCells":
					if mergesSeen {
						return fmt.Errorf("duplicate XLSX mergeCells")
					}
					mergesSeen, inMerges = true, true
				default:
					if err := d.Skip(); err != nil {
						return err
					}
				}
			case xml.EndElement:
				switch t.Name.Local {
				case "sheetData":
					inData = false
				case "mergeCells":
					inMerges = false
				case "worksheet":
					closed = true
				}
			case xml.CharData:
				if strings.TrimSpace(string(t)) != "" {
					return fmt.Errorf("unexpected XLSX structure text")
				}
			}
		}
	})
	if maxCol >= 0 {
		l.Extent = fmt.Sprintf("%s%d:%s%d", xlsxColumnName(minCol), minRow, xlsxColumnName(maxCol), maxRow)
	}
	return l, err
}

func scanXLSXRowShape(ctx context.Context, d *xml.Decoder, start xml.StartElement, n int) (XLSXRowShape, int, int, error) {
	out := XLSXRowShape{Number: n}
	first, last := -1, -1
	for {
		if err := ctx.Err(); err != nil {
			return out, first, last, err
		}
		t, err := d.Token()
		if err != nil {
			return out, first, last, err
		}
		switch t := t.(type) {
		case xml.StartElement:
			if t.Name.Local != "c" {
				return out, first, last, fmt.Errorf("unexpected XLSX row child")
			}
			c, ok := strictXLSXCoordinate(xlsxAttribute(t.Attr, "r"))
			if !ok || c.Row != n || c.Column <= last {
				return out, first, last, fmt.Errorf("invalid or duplicate XLSX cell position")
			}
			if first < 0 {
				first = c.Column
				out.First = xlsxColumnName(first)
			}
			last = c.Column
			out.Last = xlsxColumnName(last)
			out.Cells++
			if err := d.Skip(); err != nil {
				return out, first, last, err
			}
		case xml.EndElement:
			if t.Name == start.Name {
				return out, first, last, nil
			}
		case xml.CharData:
			if strings.TrimSpace(string(t)) != "" {
				return out, first, last, fmt.Errorf("unexpected XLSX row text")
			}
		}
	}
}
