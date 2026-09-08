package dataset

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

const maxFileVersions = 32

// FileVersion is a portal-advertised historical edition, not a content hash or
// an assertion about record validity. IDs must be selected from a fresh listing.
type FileVersion struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	RegisteredAt string `json:"registeredAt,omitempty"`
	detailPK     string
	historySN    string
}

var historyDetailPK = regexp.MustCompile(`^uddi:[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var historySerial = regexp.MustCompile(`^[1-9][0-9]{0,8}$`)
var historyAttachment = regexp.MustCompile(`^FILE_[A-Za-z0-9_-]{1,100}$`)
var historicalDownload = regexp.MustCompile(`^fn_fileDataDown\(\s*'([^']*)'\s*,\s*'([^']*)'\s*,\s*'([^']*)'\s*,\s*'([^']*)'\s*,\s*'([^']*)'\s*\)\s*;?$`)

// InspectHistory lists a bounded prefix of advertised editions when version is
// empty. Otherwise it revalidates membership and resolves only that edition.
// It never falls back to the newest file or accepts a caller-built URL.
func (i *Inspector) InspectHistory(ctx context.Context, ref Ref, version string) (*Contract, error) {
	if len(version) > 100 {
		return nil, fmt.Errorf("FILE version ID exceeds 100 bytes")
	}
	return i.inspect(ctx, ref, true, version)
}

func (i *Inspector) historyDocument(ctx context.Context, path string, form url.Values, limit int) (*goquery.Document, Evidence, error) {
	r := Request{Method: http.MethodPost, URL: i.base + path, Form: form}
	evidence := Evidence{Kind: EvidenceFirstPartyWebContract, URL: r.URL, Stability: StabilityFallback, Request: &r}
	res, err := i.http.PostForm(ctx, r.URL, form)
	if err != nil {
		return nil, evidence, err
	}
	if res.Status != http.StatusOK || len(res.Body) == 0 || len(res.Body) > limit {
		return nil, evidence, fmt.Errorf("portal FILE history response invalid or exceeds bounded size (HTTP %d)", res.Status)
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(res.Body))
	return doc, evidence, err
}

func (i *Inspector) inspectHistory(ctx context.Context, latest *Contract, download []string, version string) (*Contract, error) {
	if download[1] != latest.Ref.PK || !historyDetailPK.MatchString(download[2]) {
		return nil, fmt.Errorf("portal FILE history dataset identity mismatch")
	}
	doc, listing, err := i.historyDocument(ctx, "/tcs/dss/selectHistAndCsvData.do", url.Values{
		"publicDataPk": {latest.Ref.PK}, "publicDataDetailPk": {download[2]},
	}, 8<<20)
	if err != nil {
		return nil, err
	}
	section := doc.Find("#tab-layer-file-05")
	if section.Length() != 1 {
		return nil, fmt.Errorf("portal FILE history section missing or ambiguous; history coverage unknown")
	}
	listing.Purpose = "file-version-membership"
	latest.Evidence = append(latest.Evidence, listing)
	entries := section.Find("a.openFileDetailPopup")
	count := section.Find("h3 span")
	declared, countErr := strconv.Atoi(strings.TrimSpace(count.Text()))
	if count.Length() != 1 || countErr != nil || declared != entries.Length() || declared < 0 {
		return nil, fmt.Errorf("portal FILE history count disagrees with recognized editions; history coverage unknown")
	}
	latest.FileHistoryCount = entries.Length()
	latest.FileHistoryTruncated = entries.Length() > maxFileVersions
	seen := map[string]bool{}
	var parseErr error
	entries.EachWithBreak(func(index int, a *goquery.Selection) bool {
		if index >= maxFileVersions {
			return false
		}
		dpk, _ := a.Attr("data-public-pk")
		sn, _ := a.Attr("data-public-detail-sn")
		name := strings.Join(strings.Fields(a.Text()), " ")
		date := strings.TrimSpace(a.Closest("tr").Find("td").Last().Text())
		id := dpk + "/" + sn
		if !historyDetailPK.MatchString(dpk) || !historySerial.MatchString(sn) || name == "" || len(name) > 400 || len(date) > 100 || seen[id] {
			parseErr = fmt.Errorf("portal FILE history has invalid or duplicate edition metadata")
			return false
		}
		seen[id] = true
		latest.FileVersions = append(latest.FileVersions, FileVersion{ID: id, Name: name, RegisteredAt: date, detailPK: dpk, historySN: sn})
		return true
	})
	if parseErr != nil {
		return nil, parseErr
	}
	if version == "" {
		latest.Warnings = append(latest.Warnings, "FILE history lists at most 32 portal-advertised editions in portal order; no file was resolved or downloaded. Registration dates are not record reference dates. Select an exact fileVersion ID to inspect its contract.")
		return latest, nil
	}
	for _, entry := range latest.FileVersions {
		if entry.ID == version {
			return i.inspectHistoricalEdition(ctx, latest, entry, listing)
		}
	}
	return nil, fmt.Errorf("FILE version is not in the bounded advertised history; no latest-file fallback")
}

func (i *Inspector) inspectHistoricalEdition(ctx context.Context, latest *Contract, version FileVersion, listing Evidence) (*Contract, error) {
	doc, evidence, err := i.historyDocument(ctx, "/tcs/dss/selectDpkDetailInfo.do", url.Values{
		"publicDataDetailPk": {version.detailPK}, "publicDataHistSn": {version.historySN},
	}, 2<<20)
	if err != nil {
		return nil, err
	}
	popup := doc.Find("#file-detail-popup")
	if popup.Length() != 1 {
		return nil, fmt.Errorf("historical FILE detail popup missing or ambiguous")
	}
	metadata := fileMetadata(goquery.NewDocumentFromNode(popup.Get(0)))
	evidence.Purpose = "historical-file-contract"
	c := &Contract{Ref: latest.Ref, Name: version.Name, Provider: metadata["제공기관"], UpdateCycle: metadata["업데이트 주기"], ModifiedAt: metadata["수정일"], DeclaredFormat: strings.ToUpper(metadata["확장자"]), SourceURL: latest.SourceURL, AdapterID: "datagokr-file", AdapterRevision: 1, VerifiedAt: "2026-09-08", Capability: CapabilityInspectable, Metadata: metadata, SelectedFileVersion: &version, Evidence: []Evidence{listing, evidence}, Warnings: []string{"Historical FILE metadata is from the selected portal popup only; absent declarations are unknown. Current-edition metadata is not inherited. Version labels and registration dates are not proof of record validity."}}
	c.FileVersions, c.FileHistoryCount, c.FileHistoryTruncated = latest.FileVersions, latest.FileHistoryCount, latest.FileHistoryTruncated
	var links [][]string
	formats := map[string]string{}
	var parseErr error
	popup.Find("[onclick]").Each(func(_ int, a *goquery.Selection) {
		value, _ := a.Attr("onclick")
		m := historicalDownload.FindStringSubmatch(strings.TrimSpace(value))
		if m == nil {
			return
		}
		if m[1] != c.Ref.PK || m[2] != version.detailPK || !historyAttachment.MatchString(m[3]) || !historySerial.MatchString(m[4]) {
			parseErr = fmt.Errorf("historical FILE download identity differs from the advertised edition")
			return
		}
		key := m[3] + "/" + m[4]
		format := strings.ToUpper(m[5])
		if previous, exists := formats[key]; exists {
			if previous != format {
				parseErr = fmt.Errorf("historical FILE download buttons disagree on attachment format")
			}
			return
		}
		formats[key] = format
		links = append(links, m)
	})
	if parseErr != nil {
		return nil, parseErr
	}
	if len(links) == 0 || len(links) > 8 {
		return nil, fmt.Errorf("historical FILE edition requires 1–8 explicit download contracts")
	}
	assetNames := map[string]bool{}
	for _, m := range links {
		a, err := i.resolvePortalAsset(ctx, m)
		if err != nil {
			return nil, err
		}
		if a.Name == "" || len(a.Name) > 512 || assetNames[a.Name] {
			return nil, fmt.Errorf("historical FILE asset names must be bounded and unambiguous")
		}
		assetNames[a.Name] = true
		u, err := url.Parse(a.Request.URL)
		if err != nil || u.Query().Get("atchFileId") != m[3] || u.Query().Get("fileDetailSn") != m[4] {
			return nil, fmt.Errorf("historical FILE resolution changed the requested attachment")
		}
		format := strings.ToUpper(m[5])
		if !strings.EqualFold(path.Ext(a.Name), "."+format) {
			return nil, fmt.Errorf("historical FILE filename disagrees with its advertised download format")
		}
		if a.Format != format {
			c.Warnings = append(c.Warnings, fmt.Sprintf("Historical asset %q: resolver extension %q differs from advertised button and filename %q; using the matching button/filename format, not validating file contents.", a.Name, a.Format, format))
		}
		a.Format = format
		c.Assets = append(c.Assets, a)
	}
	c.Capability = CapabilityRetrievable
	return c, nil
}
