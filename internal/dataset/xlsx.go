package dataset

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const (
	maxXLSXColumns     = 256
	maxXLSXRowsScanned = 64
	maxXLSXHeaderRows  = 12
)

type xlsxWorkbook struct {
	Sheets []xlsxSheet `xml:"sheets>sheet"`
}

type xlsxSheet struct {
	Name           string `xml:"name,attr"`
	RelationshipID string `xml:"id,attr"`
}

type xlsxRelationships struct {
	Items []xlsxRelationship `xml:"Relationship"`
}

type xlsxRelationship struct {
	ID     string `xml:"Id,attr"`
	Target string `xml:"Target,attr"`
	Type   string `xml:"Type,attr"`
}

type xlsxSharedStrings struct {
	Items []xlsxRichText `xml:"si"`
}

type xlsxRichText struct {
	Text string `xml:"t"`
	Runs []struct {
		Text string `xml:"t"`
	} `xml:"r"`
}

func (r xlsxRichText) value() string {
	if r.Text != "" {
		return r.Text
	}
	var out strings.Builder
	for _, run := range r.Runs {
		out.WriteString(run.Text)
	}
	return out.String()
}

type xlsxWorksheet struct {
	Rows   []xlsxRow `xml:"sheetData>row"`
	Merges []struct {
		Ref string `xml:"ref,attr"`
	} `xml:"mergeCells>mergeCell"`
}

type xlsxRow struct {
	Number int        `xml:"r,attr"`
	Cells  []xlsxCell `xml:"c"`
}

type xlsxCell struct {
	Ref    string       `xml:"r,attr"`
	Type   string       `xml:"t,attr"`
	Value  string       `xml:"v"`
	Inline xlsxRichText `xml:"is"`
}

type xlsxCoordinate struct {
	Column int
	Row    int
}

// inspectXLSX reads SpreadsheetML directly from the bounded ZIP container. It
// does not evaluate formulas, macros, external links, or extract any member to
// disk; cached cell values are enough to profile worksheet columns.
func inspectXLSX(reader io.ReaderAt, size int64) ([]ObservedFile, []string, error) {
	zr, err := zip.NewReader(reader, size)
	if err != nil {
		return nil, nil, fmt.Errorf("XLSX ZIP 해석 실패: %w", err)
	}
	if len(zr.File) > maxArchiveEntries {
		return nil, nil, fmt.Errorf("XLSX 항목이 허용 개수 %d개를 초과했습니다", maxArchiveEntries)
	}
	var expanded uint64
	members := make(map[string]*zip.File, len(zr.File))
	for _, member := range zr.File {
		expanded += member.UncompressedSize64
		if expanded > maxExpandedBytes {
			return nil, nil, fmt.Errorf("XLSX 해제 크기가 허용 한도 %d bytes를 초과했습니다", maxExpandedBytes)
		}
		members[path.Clean(strings.TrimPrefix(member.Name, "/"))] = member
	}

	var workbook xlsxWorkbook
	if err := decodeXLSXMember(members["xl/workbook.xml"], &workbook); err != nil {
		return nil, nil, fmt.Errorf("XLSX workbook 해석 실패: %w", err)
	}
	var relationships xlsxRelationships
	if member := members["xl/_rels/workbook.xml.rels"]; member != nil {
		if err := decodeXLSXMember(member, &relationships); err != nil {
			return nil, nil, fmt.Errorf("XLSX workbook 관계 해석 실패: %w", err)
		}
	}
	targets := make(map[string]string, len(relationships.Items))
	for _, relationship := range relationships.Items {
		if !strings.HasSuffix(strings.ToLower(relationship.Type), "/worksheet") {
			continue
		}
		target := strings.TrimPrefix(strings.TrimSpace(relationship.Target), "/")
		if !strings.HasPrefix(target, "xl/") {
			target = path.Join("xl", target)
		}
		target = path.Clean(target)
		if strings.HasPrefix(target, "xl/worksheets/") {
			targets[relationship.ID] = target
		}
	}

	var shared xlsxSharedStrings
	if member := members["xl/sharedStrings.xml"]; member != nil {
		if err := decodeXLSXMember(member, &shared); err != nil {
			return nil, nil, fmt.Errorf("XLSX shared strings 해석 실패: %w", err)
		}
	}
	sharedValues := make([]string, len(shared.Items))
	for index, item := range shared.Items {
		sharedValues[index] = strings.TrimSpace(item.value())
	}

	var files []ObservedFile
	var warnings []string
	for index, sheet := range workbook.Sheets {
		target := targets[sheet.RelationshipID]
		if target == "" {
			// Some minimal generators omit workbook relationships. Falling back by
			// ordinal is deterministic and remains inside xl/worksheets.
			target = fmt.Sprintf("xl/worksheets/sheet%d.xml", index+1)
		}
		member := members[target]
		if member == nil {
			warnings = append(warnings, fmt.Sprintf("XLSX worksheet %q 파일을 찾지 못했습니다", sheet.Name))
			continue
		}
		var worksheet xlsxWorksheet
		if err := decodeXLSXMember(member, &worksheet); err != nil {
			warnings = append(warnings, fmt.Sprintf("XLSX worksheet %q 해석 실패: %v", sheet.Name, err))
			continue
		}
		observed, ok := observeXLSXWorksheet(sheet.Name, target, worksheet, sharedValues)
		if ok {
			files = append(files, observed)
		}
	}
	if len(files) == 0 {
		warnings = append(warnings, "XLSX에서 표 형태의 worksheet 스키마를 찾지 못했습니다")
	}
	return files, warnings, nil
}

func decodeXLSXMember(member *zip.File, target any) error {
	if member == nil {
		return fmt.Errorf("필수 XML member가 없음")
	}
	if member.UncompressedSize64 > maxExpandedBytes {
		return fmt.Errorf("XML member가 허용 한도 %d bytes를 초과했습니다", maxExpandedBytes)
	}
	rc, err := member.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	limited := &io.LimitedReader{R: rc, N: int64(member.UncompressedSize64) + 1}
	if err := xml.NewDecoder(limited).Decode(target); err != nil {
		return err
	}
	if limited.N == 0 {
		return fmt.Errorf("XML member의 실제 크기가 선언 크기를 초과했습니다")
	}
	return nil
}

func observeXLSXWorksheet(name, memberName string, sheet xlsxWorksheet, shared []string) (ObservedFile, bool) {
	raw := map[xlsxCoordinate]string{}
	rowNumbers := make([]int, 0, len(sheet.Rows))
	maxColumn := -1
	for rowIndex, row := range sheet.Rows {
		if rowIndex >= maxXLSXRowsScanned {
			break
		}
		rowNumber := row.Number
		if rowNumber <= 0 {
			rowNumber = rowIndex + 1
		}
		rowNumbers = append(rowNumbers, rowNumber)
		for _, cell := range row.Cells {
			coordinate, ok := parseXLSXCoordinate(cell.Ref)
			if !ok || coordinate.Column >= maxXLSXColumns {
				continue
			}
			if coordinate.Row <= 0 {
				coordinate.Row = rowNumber
			}
			value := xlsxCellValue(cell, shared)
			if value != "" {
				raw[coordinate] = value
			}
			if coordinate.Column > maxColumn {
				maxColumn = coordinate.Column
			}
		}
	}
	if maxColumn < 0 || len(rowNumbers) == 0 {
		return ObservedFile{}, false
	}
	sort.Ints(rowNumbers)

	expanded := make(map[xlsxCoordinate]string, len(raw))
	for coordinate, value := range raw {
		expanded[coordinate] = value
	}
	for _, merge := range sheet.Merges {
		start, end, ok := parseXLSXRange(merge.Ref)
		if !ok || start.Column >= maxXLSXColumns || start.Row > rowNumbers[len(rowNumbers)-1] {
			continue
		}
		value := raw[start]
		if value == "" {
			continue
		}
		if end.Column >= maxXLSXColumns {
			end.Column = maxXLSXColumns - 1
		}
		for row := start.Row; row <= end.Row; row++ {
			for column := start.Column; column <= end.Column; column++ {
				expanded[xlsxCoordinate{Column: column, Row: row}] = value
			}
		}
		if end.Column > maxColumn {
			maxColumn = end.Column
		}
	}

	headerRow, headerScore := 0, 0
	for index, row := range rowNumbers {
		if index >= maxXLSXHeaderRows {
			break
		}
		score := 0
		for column := 0; column <= maxColumn; column++ {
			if value := raw[xlsxCoordinate{Column: column, Row: row}]; isXLSXHeaderValue(value) {
				score++
			}
		}
		if score >= headerScore && score > 0 {
			headerRow, headerScore = row, score
		}
	}
	if headerRow == 0 {
		return ObservedFile{}, false
	}

	columns := make([]string, maxColumn+1)
	for column := 0; column <= maxColumn; column++ {
		var parts []string
		for _, row := range []int{headerRow - 1, headerRow} {
			value := strings.TrimSpace(expanded[xlsxCoordinate{Column: column, Row: row}])
			if !isXLSXHeaderValue(value) || (len(parts) > 0 && parts[len(parts)-1] == value) {
				continue
			}
			parts = append(parts, value)
		}
		if len(parts) == 0 {
			columns[column] = xlsxColumnName(column)
		} else {
			columns[column] = strings.Join(parts, " / ")
		}
	}

	sampleRows := 0
	for _, row := range rowNumbers {
		if row <= headerRow {
			continue
		}
		for column := 0; column <= maxColumn; column++ {
			if strings.TrimSpace(raw[xlsxCoordinate{Column: column, Row: row}]) != "" {
				sampleRows++
				break
			}
		}
		if sampleRows == maxSampleRows {
			break
		}
	}
	if strings.TrimSpace(name) == "" {
		name = memberName
	}
	return ObservedFile{Name: name, Format: "XLSX", Columns: columns, SampleRows: sampleRows}, true
}

func xlsxCellValue(cell xlsxCell, shared []string) string {
	value := strings.TrimSpace(cell.Value)
	switch cell.Type {
	case "s":
		index, err := strconv.Atoi(value)
		if err == nil && index >= 0 && index < len(shared) {
			return shared[index]
		}
		return ""
	case "inlineStr":
		return strings.TrimSpace(cell.Inline.value())
	default:
		return value
	}
}

func parseXLSXCoordinate(value string) (xlsxCoordinate, bool) {
	value = strings.TrimSpace(value)
	column, index := 0, 0
	for index < len(value) {
		r := rune(value[index])
		if r >= 'a' && r <= 'z' {
			r -= 'a' - 'A'
		}
		if r < 'A' || r > 'Z' {
			break
		}
		column = column*26 + int(r-'A'+1)
		index++
	}
	if index == 0 || index == len(value) {
		return xlsxCoordinate{}, false
	}
	row, err := strconv.Atoi(value[index:])
	if err != nil || row <= 0 {
		return xlsxCoordinate{}, false
	}
	return xlsxCoordinate{Column: column - 1, Row: row}, true
}

func parseXLSXRange(value string) (xlsxCoordinate, xlsxCoordinate, bool) {
	parts := strings.SplitN(value, ":", 2)
	start, ok := parseXLSXCoordinate(parts[0])
	if !ok {
		return xlsxCoordinate{}, xlsxCoordinate{}, false
	}
	if len(parts) == 1 {
		return start, start, true
	}
	end, ok := parseXLSXCoordinate(parts[1])
	if !ok || end.Column < start.Column || end.Row < start.Row {
		return xlsxCoordinate{}, xlsxCoordinate{}, false
	}
	return start, end, true
}

func isXLSXHeaderValue(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, r := range value {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func xlsxColumnName(index int) string {
	index++
	var name []byte
	for index > 0 {
		index--
		name = append([]byte{byte('A' + index%26)}, name...)
		index /= 26
	}
	return string(name)
}
