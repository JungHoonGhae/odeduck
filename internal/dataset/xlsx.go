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
	maxXLSXColumns            = 256
	maxXLSXRowsScanned        = 64
	maxXLSXHeaderRows         = 12
	maxXLSXWorksheets         = 16
	maxXLSXRelationships      = 128
	maxXLSXMergeRanges        = 1024
	maxXLSXExpandedMergeCells = maxXLSXColumns * maxXLSXRowsScanned
	maxXLSXCellTextBytes      = 64 << 10
	maxXLSXSharedStringItems  = 1_000_000
	maxXLSXSharedStringRefs   = maxXLSXWorksheets * maxXLSXRowsScanned * maxXLSXColumns
	maxXLSXSharedStringBytes  = 8 << 20
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
	Rows   []xlsxRow
	Merges []xlsxMerge
}

type xlsxMerge struct {
	Ref string
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

	workbook, err := decodeXLSXWorkbook(members["xl/workbook.xml"])
	if err != nil {
		return nil, nil, fmt.Errorf("XLSX workbook 해석 실패: %w", err)
	}
	relationships := xlsxRelationships{}
	if member := members["xl/_rels/workbook.xml.rels"]; member != nil {
		relationships, err = decodeXLSXRelationships(member)
		if err != nil {
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

	type worksheetResult struct {
		worksheet *xlsxWorksheet
		err       error
	}
	type selectedWorksheet struct {
		sheet     xlsxSheet
		target    string
		worksheet *xlsxWorksheet
	}
	worksheetCache := make(map[string]worksheetResult)
	selected := make([]selectedWorksheet, 0, len(workbook.Sheets))
	sharedReferences := make(map[int]struct{})
	var warnings []string
	for index, sheet := range workbook.Sheets {
		target := targets[sheet.RelationshipID]
		if target == "" {
			// Some minimal generators omit workbook relationships. Falling back by
			// ordinal is deterministic and remains inside xl/worksheets.
			target = fmt.Sprintf("xl/worksheets/sheet%d.xml", index+1)
		}
		result, cached := worksheetCache[target]
		if !cached {
			member := members[target]
			if member == nil {
				result.err = fmt.Errorf("파일을 찾지 못했습니다")
			} else {
				worksheet, decodeErr := decodeXLSXWorksheet(member)
				result.worksheet = &worksheet
				if decodeErr != nil {
					result.err = fmt.Errorf("해석 실패: %w", decodeErr)
				}
			}
			worksheetCache[target] = result
		}
		if result.err != nil {
			warnings = append(warnings, fmt.Sprintf("XLSX worksheet %q %v", sheet.Name, result.err))
			continue
		}
		if err := collectXLSXSharedStringReferences(*result.worksheet, sharedReferences); err != nil {
			return nil, nil, fmt.Errorf("XLSX shared string 참조 해석 실패: %w", err)
		}
		selected = append(selected, selectedWorksheet{sheet: sheet, target: target, worksheet: result.worksheet})
	}

	sharedValues := map[int]string{}
	if member := members["xl/sharedStrings.xml"]; member != nil && len(sharedReferences) > 0 {
		sharedValues, err = decodeXLSXSharedStrings(member, sharedReferences)
		if err != nil {
			return nil, nil, fmt.Errorf("XLSX shared strings 해석 실패: %w", err)
		}
	}

	var files []ObservedFile
	for _, item := range selected {
		observed, ok := observeXLSXWorksheet(item.sheet.Name, item.target, *item.worksheet, sharedValues)
		if ok {
			files = append(files, observed)
		}
	}
	if len(files) == 0 {
		warnings = append(warnings, "XLSX에서 표 형태의 worksheet 스키마를 찾지 못했습니다")
	}
	return files, warnings, nil
}

func decodeXLSXWorkbook(member *zip.File) (xlsxWorkbook, error) {
	workbook := xlsxWorkbook{Sheets: make([]xlsxSheet, 0, maxXLSXWorksheets)}
	err := streamXLSXMember(member, func(decoder *xml.Decoder) error {
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			start, ok := token.(xml.StartElement)
			if !ok || start.Name.Local != "sheet" {
				continue
			}
			if len(workbook.Sheets) >= maxXLSXWorksheets {
				return fmt.Errorf("worksheet 개수가 허용 한도 %d개를 초과했습니다", maxXLSXWorksheets)
			}
			workbook.Sheets = append(workbook.Sheets, xlsxSheet{
				Name:           xlsxAttribute(start.Attr, "name"),
				RelationshipID: xlsxAttribute(start.Attr, "id"),
			})
			if err := decoder.Skip(); err != nil {
				return err
			}
		}
	})
	return workbook, err
}

func decodeXLSXRelationships(member *zip.File) (xlsxRelationships, error) {
	relationships := xlsxRelationships{Items: make([]xlsxRelationship, 0)}
	err := streamXLSXMember(member, func(decoder *xml.Decoder) error {
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			start, ok := token.(xml.StartElement)
			if !ok || start.Name.Local != "Relationship" {
				continue
			}
			if len(relationships.Items) >= maxXLSXRelationships {
				return fmt.Errorf("관계 개수가 허용 한도 %d개를 초과했습니다", maxXLSXRelationships)
			}
			relationships.Items = append(relationships.Items, xlsxRelationship{
				ID: xlsxAttribute(start.Attr, "Id"), Target: xlsxAttribute(start.Attr, "Target"),
				Type: xlsxAttribute(start.Attr, "Type"),
			})
			if err := decoder.Skip(); err != nil {
				return err
			}
		}
	})
	return relationships, err
}

func decodeXLSXSharedStrings(member *zip.File, wanted map[int]struct{}) (map[int]string, error) {
	values := make(map[int]string, len(wanted))
	if len(wanted) == 0 {
		return values, nil
	}
	retainedBytes := 0
	itemIndex := 0
	err := streamXLSXMember(member, func(decoder *xml.Decoder) error {
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			start, ok := token.(xml.StartElement)
			if !ok || start.Name.Local != "si" {
				continue
			}
			if itemIndex >= maxXLSXSharedStringItems {
				return fmt.Errorf("shared string 개수가 허용 한도 %d개를 초과했습니다", maxXLSXSharedStringItems)
			}
			if _, keep := wanted[itemIndex]; keep {
				value, err := decodeBoundedXLSXInlineString(decoder, start)
				if err != nil {
					return err
				}
				retainedBytes += len(value)
				if retainedBytes > maxXLSXSharedStringBytes {
					return fmt.Errorf("shared string 보관 크기가 허용 한도 %d bytes를 초과했습니다", maxXLSXSharedStringBytes)
				}
				values[itemIndex] = strings.TrimSpace(value)
			} else if err := decoder.Skip(); err != nil {
				return err
			}
			itemIndex++
		}
	})
	return values, err
}

func collectXLSXSharedStringReferences(worksheet xlsxWorksheet, references map[int]struct{}) error {
	for _, row := range worksheet.Rows {
		for _, cell := range row.Cells {
			if cell.Type != "s" {
				continue
			}
			index, err := strconv.Atoi(strings.TrimSpace(cell.Value))
			if err != nil || index < 0 {
				continue
			}
			if index >= maxXLSXSharedStringItems {
				return fmt.Errorf("shared string index %d가 허용 한도를 초과했습니다", index)
			}
			if _, exists := references[index]; exists {
				continue
			}
			if len(references) >= maxXLSXSharedStringRefs {
				return fmt.Errorf("shared string 참조가 허용 한도 %d개를 초과했습니다", maxXLSXSharedStringRefs)
			}
			references[index] = struct{}{}
		}
	}
	return nil
}

func streamXLSXMember(member *zip.File, consume func(*xml.Decoder) error) error {
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
	if err := consume(xml.NewDecoder(limited)); err != nil {
		return err
	}
	if limited.N == 0 {
		return fmt.Errorf("XML member의 실제 크기가 선언 크기를 초과했습니다")
	}
	return nil
}

// decodeXLSXWorksheet streams worksheet XML so the observation limits apply
// while parsing, before untrusted row and cell counts can become heap objects.
func decodeXLSXWorksheet(member *zip.File) (xlsxWorksheet, error) {
	worksheet := xlsxWorksheet{
		Rows:   make([]xlsxRow, 0, maxXLSXRowsScanned),
		Merges: make([]xlsxMerge, 0),
	}
	err := streamXLSXMember(member, func(decoder *xml.Decoder) error {
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			start, ok := token.(xml.StartElement)
			if !ok {
				continue
			}
			switch start.Name.Local {
			case "row":
				if len(worksheet.Rows) >= maxXLSXRowsScanned {
					if err := decoder.Skip(); err != nil {
						return err
					}
					continue
				}
				row, err := decodeXLSXRow(decoder, start)
				if err != nil {
					return err
				}
				worksheet.Rows = append(worksheet.Rows, row)
			case "mergeCell":
				if len(worksheet.Merges) < maxXLSXMergeRanges {
					worksheet.Merges = append(worksheet.Merges, xlsxMerge{Ref: xlsxAttribute(start.Attr, "ref")})
				}
				if err := decoder.Skip(); err != nil {
					return err
				}
			}
		}
	})
	return worksheet, err
}

func decodeXLSXRow(decoder *xml.Decoder, start xml.StartElement) (xlsxRow, error) {
	row := xlsxRow{}
	row.Number, _ = strconv.Atoi(xlsxAttribute(start.Attr, "r"))
	row.Cells = make([]xlsxCell, 0, maxXLSXColumns)
	for {
		token, err := decoder.Token()
		if err != nil {
			return xlsxRow{}, err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			if typed.Name.Local != "c" {
				continue
			}
			coordinate, valid := parseXLSXCoordinate(xlsxAttribute(typed.Attr, "r"))
			if !valid || coordinate.Column >= maxXLSXColumns || len(row.Cells) >= maxXLSXColumns {
				if err := decoder.Skip(); err != nil {
					return xlsxRow{}, err
				}
				continue
			}
			cell, err := decodeXLSXCell(decoder, typed)
			if err != nil {
				return xlsxRow{}, err
			}
			row.Cells = append(row.Cells, cell)
		case xml.EndElement:
			if typed.Name == start.Name {
				return row, nil
			}
		}
	}
}

func decodeXLSXCell(decoder *xml.Decoder, start xml.StartElement) (xlsxCell, error) {
	cell := xlsxCell{Ref: xlsxAttribute(start.Attr, "r"), Type: xlsxAttribute(start.Attr, "t")}
	for {
		token, err := decoder.Token()
		if err != nil {
			return xlsxCell{}, err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			switch typed.Name.Local {
			case "v":
				cell.Value, err = decodeBoundedXLSXText(decoder, typed)
				if err != nil {
					return xlsxCell{}, err
				}
			case "is":
				cell.Inline.Text, err = decodeBoundedXLSXInlineString(decoder, typed)
				if err != nil {
					return xlsxCell{}, err
				}
			default:
				if err := decoder.Skip(); err != nil {
					return xlsxCell{}, err
				}
			}
		case xml.EndElement:
			if typed.Name == start.Name {
				return cell, nil
			}
		}
	}
}

func decodeBoundedXLSXInlineString(decoder *xml.Decoder, start xml.StartElement) (string, error) {
	var value strings.Builder
	for {
		token, err := decoder.Token()
		if err != nil {
			return "", err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			if typed.Name.Local != "t" {
				continue
			}
			part, err := decodeBoundedXLSXText(decoder, typed)
			if err != nil {
				return "", err
			}
			if value.Len()+len(part) > maxXLSXCellTextBytes {
				return "", fmt.Errorf("XLSX cell text가 허용 한도 %d bytes를 초과했습니다", maxXLSXCellTextBytes)
			}
			value.WriteString(part)
		case xml.EndElement:
			if typed.Name == start.Name {
				return value.String(), nil
			}
		}
	}
}

func decodeBoundedXLSXText(decoder *xml.Decoder, start xml.StartElement) (string, error) {
	var value strings.Builder
	depth := 1
	for depth > 0 {
		token, err := decoder.Token()
		if err != nil {
			return "", err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			if value.Len()+len(typed) > maxXLSXCellTextBytes {
				return "", fmt.Errorf("XLSX cell text가 허용 한도 %d bytes를 초과했습니다", maxXLSXCellTextBytes)
			}
			value.Write(typed)
		}
	}
	return value.String(), nil
}

func xlsxAttribute(attributes []xml.Attr, localName string) string {
	for _, attribute := range attributes {
		if attribute.Name.Local == localName {
			return attribute.Value
		}
	}
	return ""
}

func observeXLSXWorksheet(name, memberName string, sheet xlsxWorksheet, shared map[int]string) (ObservedFile, bool) {
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
	expandedMergeCells := 0
mergeLoop:
	for _, merge := range sheet.Merges {
		start, end, ok := boundedXLSXMergeRange(merge.Ref, rowNumbers[len(rowNumbers)-1])
		if !ok {
			continue
		}
		value := raw[start]
		if value == "" {
			continue
		}
		for _, row := range rowNumbers {
			if row < start.Row || row > end.Row {
				continue
			}
			for column := start.Column; column <= end.Column; column++ {
				if expandedMergeCells >= maxXLSXExpandedMergeCells {
					break mergeLoop
				}
				expanded[xlsxCoordinate{Column: column, Row: row}] = value
				expandedMergeCells++
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

func boundedXLSXMergeRange(value string, maxObservedRow int) (xlsxCoordinate, xlsxCoordinate, bool) {
	start, end, ok := parseXLSXRange(value)
	if !ok || maxObservedRow <= 0 || start.Column >= maxXLSXColumns || start.Row > maxObservedRow {
		return xlsxCoordinate{}, xlsxCoordinate{}, false
	}
	if end.Column >= maxXLSXColumns {
		end.Column = maxXLSXColumns - 1
	}
	if end.Row > maxObservedRow {
		end.Row = maxObservedRow
	}
	return start, end, true
}

func xlsxCellValue(cell xlsxCell, shared map[int]string) string {
	value := strings.TrimSpace(cell.Value)
	switch cell.Type {
	case "s":
		index, err := strconv.Atoi(value)
		if err == nil && index >= 0 {
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
