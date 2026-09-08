package dataset

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"github.com/PuerkitoBio/goquery"
)

const monthlyExportID = "mois-monthly-age-csv"
const monthlyExportURL = "https://jumin.mois.go.kr/downloadCsvAge.do?searchYearMonth=month&xlsStats=3"
const MonthlyExportSelectionMode = "mois_province_code_full_scan_v1"

// ExportReference advertises a registered operation, not a downloadable asset.
// Parameters describes its typed caller choices; raw form fields are private.
type ExportReference struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	AdvertisedURL   string `json:"advertisedUrl"`
	DiscoveryURL    string `json:"discoveryUrl"`
	AdapterID       string `json:"adapterId"`
	AdapterRevision int    `json:"adapterRevision"`
	VerifiedAt      string `json:"verifiedAt"`
	Parameters      string `json:"parameters"`
}

type ExportChoices struct {
	Month        string `json:"month"`
	Registration string `json:"registration"`
	ProvinceCode string `json:"provinceCode"`
	AgeFrom      int    `json:"ageFrom"`
	AgeTo        int    `json:"ageTo"`
}

// ExportProvenance ties a complete CSV to a selected official form. It does not
// assert that a local province-code selection covers the question's population.
type ExportProvenance struct {
	Reference  ExportReference `json:"reference"`
	Choices    ExportChoices   `json:"choices"`
	PageSHA256 string          `json:"pageSha256"`
	PageBytes  int             `json:"pageBytes"`
	Request    Request         `json:"request"`
	Columns    []string        `json:"columns"`
	CodeField  string          `json:"codeField"`
	CodeRule   string          `json:"codeRule"`
	CodePrefix string          `json:"codePrefix"`
}

func fileExports(source, discovery string) []ExportReference {
	if source != monthlyDocumentURL {
		return nil
	}
	return []ExportReference{{ID: monthlyExportID, Title: "월간 행정동별 연령별 인구 CSV", AdvertisedURL: source, DiscoveryURL: discovery, AdapterID: "mois-monthly-export", AdapterRevision: 1, VerifiedAt: "2026-09-08", Parameters: "Required string params: month YYYY-MM (2008 onward, actual publication checked), registration all|resident|unknown|overseas, provinceCode ten-digit province code, ageFrom and ageTo inclusive integers 0–100 (100 means 100+). At most 83 ages per request under the existing column cap. One-year groups. Complete national CSV scan with local province-code selection; aggregates/zero/branch rows retained, not population approval."}}
}

var provinceCodePattern = regexp.MustCompile(`^[0-9]{2}00000000$`)
var exportedCodePattern = regexp.MustCompile(`\(([0-9]{10})\)$`)

// ValidateExportSelection checks typed choices, not inspection authorization.
func ValidateExportSelection(operation string, params map[string]string) error {
	_, err := parseExportChoices(operation, params)
	return err
}

func parseExportChoices(operation string, params map[string]string) (ExportChoices, error) {
	c := ExportChoices{Month: params["month"], Registration: params["registration"], ProvinceCode: params["provinceCode"]}
	if operation != monthlyExportID || len(params) != 5 {
		return c, fmt.Errorf("select a registered FILE export and its five required typed parameters")
	}
	for _, key := range []string{"month", "registration", "provinceCode", "ageFrom", "ageTo"} {
		if value, ok := params[key]; !ok || value == "" || len(value) > 64 {
			return c, fmt.Errorf("export requires bounded %s", key)
		}
	}
	month, err := time.Parse("2006-01", c.Month)
	if err != nil || month.Year() < 2008 || !provinceCodePattern.MatchString(c.ProvinceCode) || c.ProvinceCode == "0000000000" || !slices.Contains([]string{"all", "resident", "unknown", "overseas"}, c.Registration) {
		return c, fmt.Errorf("export requires a month from 2008, a province code and a supported registration choice")
	}
	for key, target := range map[string]*int{"ageFrom": &c.AgeFrom, "ageTo": &c.AgeTo} {
		v, err := strconv.Atoi(params[key])
		if err != nil || strconv.Itoa(v) != params[key] || v < 0 || v > 100 {
			return c, fmt.Errorf("export ages must be canonical integers 0–100; 100 is the publisher's 100+ bin")
		}
		*target = v
	}
	if c.AgeTo < c.AgeFrom || c.AgeTo-c.AgeFrom+1 > 83 {
		return c, fmt.Errorf("export requires an ordered range of at most 83 age columns under the observation column cap; do not drop requested ages")
	}
	return c, nil
}

func (c ExportChoices) form() url.Values {
	registration := map[string]string{"all": "", "resident": "Y", "unknown": "N", "overseas": "O"}[c.Registration]
	return url.Values{"sltOrgType": {"2"}, "sltOrgLvl1": {c.ProvinceCode}, "sltOrgLvl2": {"A"}, "sltUndefType": {registration}, "searchYearStart": {c.Month[:4]}, "searchYearEnd": {c.Month[:4]}, "searchMonthStart": {c.Month[5:]}, "searchMonthEnd": {c.Month[5:]}, "sum": {"sum"}, "gender": {"gender"}, "sltOrderType": {"1"}, "sltOrderValue": {"ASC"}, "sltArgTypes": {"1"}, "sltArgTypeA": {strconv.Itoa(c.AgeFrom)}, "sltArgTypeB": {strconv.Itoa(c.AgeTo)}, "category": {"month"}, "state": {"3"}}
}

func (c ExportChoices) columns() []string {
	columns := []string{"행정구역"}
	for _, sex := range []string{"계", "남", "여"} {
		prefix := c.Month[:4] + "년" + c.Month[5:] + "월_" + sex + "_"
		columns = append(columns, prefix+"총인구수", prefix+"연령구간인구수")
		for age := c.AgeFrom; age <= c.AgeTo; age++ {
			name := strconv.Itoa(age) + "세"
			if age == 100 {
				name += " 이상"
			}
			columns = append(columns, prefix+name)
		}
	}
	return columns
}

// SampleExport owns form verification and complete CSV scanning. Reconstructed
// public contracts cannot create or replace its private inspection reference.
func (i *Inspector) SampleExport(ctx context.Context, c *Contract, operation string, params map[string]string, limit int) (TableSample, error) {
	choices, err := parseExportChoices(operation, params)
	if err != nil {
		return TableSample{}, err
	}
	if limit < 1 || limit > 1000 {
		return TableSample{}, fmt.Errorf("export row limit must be 1–1000")
	}
	var ref ExportReference
	if c != nil {
		for _, candidate := range c.exportReferences {
			if candidate.ID == operation {
				ref = candidate
			}
		}
	}
	if ref.ID == "" {
		return TableSample{}, fmt.Errorf("export reference was not issued by inspection")
	}
	transport, ok := i.http.(interface {
		OpenPublicPostFormNoRedirect(context.Context, string, string, url.Values) (*fetch.StreamResponse, error)
	})
	if !ok {
		return TableSample{}, fmt.Errorf("export requires credentialless no-redirect form transport")
	}
	ctx, cancel := context.WithTimeout(ctx, 55*time.Second)
	defer cancel()
	form, pageForm := choices.form(), choices.form()
	pageForm.Del("category")
	pageForm.Del("state")
	pageForm.Set("tableChart", "T")
	pageForm.Set("searchYearMonth", "month")
	page, err := transport.OpenPublicPostFormNoRedirect(ctx, monthlyDocumentURL, monthlyDocumentURL, pageForm)
	if err != nil {
		return TableSample{}, err
	}
	body, err := readExportPage(page)
	if err != nil {
		return TableSample{}, err
	}
	if err := verifyExportForm(body, form); err != nil {
		return TableSample{}, err
	}
	res, err := transport.OpenPublicPostFormNoRedirect(ctx, monthlyExportURL, monthlyDocumentURL, form)
	if err != nil {
		return TableSample{}, err
	}
	defer res.Body.Close()
	media, _, err := mime.ParseMediaType(res.ContentType)
	if err != nil || !slices.Contains([]string{"application/octet-stream", "text/csv", "application/csv"}, media) {
		return TableSample{}, fmt.Errorf("export requires a CSV download, not HTML or another response")
	}
	out := TableSample{CSV: &CSVProvenance{}}
	retainedBytes, matches := 2, 0
	report, err := scanCSVResponse(ctx, res, nil, func(record CSVScanRecord) error {
		code := exportedCodePattern.FindStringSubmatch(record.Values["행정구역"])
		if len(code) != 2 {
			return fmt.Errorf("export administrative code drift at data record %d", record.DataRecord)
		}
		if !strings.HasPrefix(code[1], choices.ProvinceCode[:2]) {
			return nil
		}
		matches++
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
			return fmt.Errorf("export retained rows exceed 2 MiB; preserve goal scope when selecting observations")
		}
		out.Rows = append(out.Rows, row)
		out.CSV.DataRecords = append(out.CSV.DataRecords, record.DataRecord)
		out.CSV.StartLines = append(out.CSV.StartLines, record.StartLine)
		return nil
	})
	if err != nil {
		return TableSample{}, err
	}
	if !slices.Equal(report.Columns, choices.columns()) {
		return TableSample{}, fmt.Errorf("export month/age/sex header differs from selected contract")
	}
	if len(out.Rows) == 0 {
		return TableSample{}, fmt.Errorf("export matched no province rows; not population absence evidence")
	}
	out.SHA256, out.Bytes, out.Prefix = report.SHA256, report.Bytes, matches > len(out.Rows)
	out.CSV.Encoding = report.Encoding
	out.Selection = &SelectionReport{Mode: MonthlyExportSelectionMode, ScannedRows: report.ScannedRows, MatchedRows: matches, ReturnedRows: len(out.Rows), Exhausted: report.Exhausted}
	out.CSV.Export = &ExportProvenance{Reference: ref, Choices: choices, PageSHA256: fmt.Sprintf("%x", sha256.Sum256(body)), PageBytes: len(body), Request: Request{Method: "POST", URL: monthlyExportURL, Form: form}, Columns: report.Columns, CodeField: "행정구역", CodeRule: "trailing-parenthesized-10-digits-v1", CodePrefix: choices.ProvinceCode[:2]}
	out.Warnings = []string{"Official monthly export: requested province may still return national rows. Complete CSV scanned; only the bounded local trailing-code prefix selection is retained. Aggregates, zero rows and branch offices are not removed or summed. Total population and selected-age interval population are different fields. Raw combined names/codes and original row positions are preserved. Scan/selection do not approve identity, applicability or population coverage. The publisher's CSV charset label is not used; the strict scanner determines encoding from original bytes."}
	return out, nil
}

func readExportPage(res *fetch.StreamResponse) ([]byte, error) {
	defer res.Body.Close()
	media, params, err := mime.ParseMediaType(res.ContentType)
	if res.Status != http.StatusOK || err != nil || media != "text/html" || (params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8")) || res.ContentLength > DocumentMaxBytes {
		return nil, fmt.Errorf("export selection requires HTTP 200 UTF-8 HTML within 1 MiB; redirects are not followed")
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, DocumentMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > DocumentMaxBytes || !utf8.Valid(body) {
		return nil, fmt.Errorf("export selection exceeds 1 MiB or contains invalid UTF-8")
	}
	return body, nil
}

func verifyExportForm(body []byte, form url.Values) error {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return err
	}
	search := doc.Find(`form[name="search"][action="ageStatMonth.do"][method="post"]`)
	export := doc.Find(`form#formXlsDown[name="formXlsDown"][method="post"]`)
	if search.Length() != 1 || export.Length() != 1 || export.Find(`input#down3[name="state"][value="3"]`).Length() != 1 || export.Find(`#csvDown`).Length() != 1 {
		return fmt.Errorf("monthly export form identity drift")
	}
	for name, values := range form {
		if name == "state" {
			continue
		}
		input := export.Find(`input[type="hidden"][name="` + name + `"]`)
		value, _ := input.Attr("value")
		if input.Length() != 1 {
			return fmt.Errorf("export form field %s missing or repeated", name)
		}
		if name == "sum" || name == "gender" {
			if value != "" || search.Find(`input[type="checkbox"][name="`+name+`"][value="`+values[0]+`"][checked]`).Length() != 1 {
				return fmt.Errorf("export selected sex/total display drift")
			}
		} else if value != values[0] {
			return fmt.Errorf("export form did not preserve selected %s", name)
		}
	}
	for _, name := range []string{"searchYearStart", "searchYearEnd", "searchMonthStart", "searchMonthEnd", "sltUndefType", "sltArgTypes"} {
		selected := search.Find(`select[name="` + name + `"] option[selected]`)
		value, _ := selected.Attr("value")
		if selected.Length() != 1 || value != form.Get(name) {
			return fmt.Errorf("export page did not select %s", name)
		}
	}
	// The observed form has no static action: the CSV button sets it. Match the
	// pinned contract; never execute page JavaScript or accept a URL from it.
	script := doc.Find("script").Text()
	if !strings.Contains(script, `$("#csvDown").click(function()`) || !strings.Contains(script, `$("#formXlsDown").attr("action", "downloadCsvAge.do?searchYearMonth="+$("input[name=category]").val()+"&xlsStats=3").submit()`) {
		return fmt.Errorf("export CSV button contract drift")
	}
	return nil
}
