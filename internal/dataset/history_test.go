package dataset_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
)

func TestInspectHistoricalFileSelectsAdvertisedVersionWithoutLatestMetadata(t *testing.T) {
	const current = "uddi:11111111-1111-1111-1111-111111111111"
	const past = "uddi:22222222-2222-2222-2222-222222222222"
	resolves := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/catalog/123/fileData.json":
			fmt.Fprint(w, `{"name":"latest","description":"LATEST_ONLY","dateModified":"2026-09-02","encodingFormat":"CSV"}`)
		case "/data/123/fileData.do":
			fmt.Fprintf(w, `<button onclick="fileDetailObj.fn_fileDataDown('123', '%s', '', '1', '1')">latest</button>`, current)
		case "/tcs/dss/selectHistAndCsvData.do":
			_ = r.ParseForm()
			if r.Method != "POST" || r.Form.Get("publicDataPk") != "123" || r.Form.Get("publicDataDetailPk") != current {
				t.Error("history query did not use the current inspected dataset")
			}
			fmt.Fprintf(w, `<div id="tab-layer-file-05"><h3>주기성 과거 데이터(<span>1</span>)</h3><table><tbody><tr><td><a class="openFileDetailPopup" data-public-pk="%s" data-public-detail-sn="2">published_20260731</a></td><td>2026-08-04</td></tr></tbody></table></div>`, past)
		case "/tcs/dss/selectDpkDetailInfo.do":
			_ = r.ParseForm()
			if r.Form.Get("publicDataDetailPk") != past || r.Form.Get("publicDataHistSn") != "2" {
				t.Error("historical popup request was guessed")
			}
			fmt.Fprintf(w, `<section id="file-detail-popup"><ul><li><strong class="key">제공기관</strong><div class="value">historical provider</div></li><li><strong class="key">수정일</strong><div class="value">2026-08-18</div></li></ul><a onclick="fn_fileDataDown('123', '%s', 'FILE_PAST', '1', 'csv')">CSV</a><a onclick="fn_fileDataDown('123', '%s', 'FILE_PAST', '2', 'json')">JSON</a>`, past, past)
			// Repeated responsive buttons with the same format are one asset.
			fmt.Fprintf(w, `<a onclick="fn_fileDataDown('123', '%s', 'FILE_PAST', '1', 'CSV')">CSV again</a></section>`, past)
			fmt.Fprint(w, `<li><strong class="key">description</strong><div class="value">LATEST_ONLY outside the selected popup</div></li>`)
		case "/tcs/dss/selectFileDataDownload.do":
			resolves++
			_ = r.ParseForm()
			if r.Form.Get("publicDataDetailPk") != past || r.Form.Get("atchFileId") != "FILE_PAST" {
				t.Error("selected version resolved a different asset")
			}
			if r.Form.Get("fileDetailSn") == "2" {
				// Observed portal behavior: the converted JSON retains the CSV
				// resolver extension. The button and filename identify the variant.
				fmt.Fprint(w, `{"status":true,"atchFileId":"FILE_PAST","fileDetailSn":"2","fileDataRegistVO":{"dataNm":"published_20260731","orginlFileNm":"past.json","atchFileExtsn":"csv"}}`)
			} else {
				fmt.Fprint(w, `{"status":true,"atchFileId":"FILE_PAST","fileDetailSn":"1","fileDataRegistVO":{"dataNm":"published_20260731","orginlFileNm":"past.csv","atchFileExtsn":"csv"}}`)
			}
		case "/cmm/cmm/fileDownload.do":
			if r.URL.Query().Get("atchFileId") != "FILE_PAST" {
				t.Error("download used the latest revision")
			}
			fmt.Fprint(w, "district,count\nA,7\n")
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer s.Close()
	i := dataset.NewInspector(fetch.New(fetch.WithDelay(0)), s.URL)
	ref := dataset.Ref{PK: "123", Delivery: "FILE"}
	list, err := i.InspectHistory(context.Background(), ref, "")
	if err != nil || len(list.FileVersions) != 1 || resolves != 0 || len(list.Assets) != 0 {
		t.Fatalf("history must list versions without resolving or downloading files: %+v %v", list, err)
	}
	version := list.FileVersions[0]
	if version.Name != "published_20260731" || version.RegisteredAt != "2026-08-04" || version.ID == "" {
		t.Fatal("version lost its publisher label or registration date")
	}
	selected, err := i.InspectHistory(context.Background(), ref, version.ID)
	if err != nil || selected.SelectedFileVersion == nil || selected.SelectedFileVersion.ID != version.ID || len(selected.Assets) != 2 {
		t.Fatalf("listed version cannot be selected: %+v %v", selected, err)
	}
	if selected.Assets[1].Format != "JSON" {
		t.Fatalf("JSON variant was falsely exposed as CSV: %+v", selected.Assets[1])
	}
	if _, err := i.SampleCSV(context.Background(), selected.Assets[1], 10); err == nil {
		t.Fatal("CSV acquisition accepted the advertised JSON variant")
	}
	if selected.Name != version.Name || selected.Provider != "historical provider" || selected.ModifiedAt != "2026-08-18" || strings.Contains(fmt.Sprint(selected.Metadata), "LATEST_ONLY") {
		t.Fatal("current metadata was incorrectly attached to a historical file")
	}
	if len(selected.FileVersions) != 1 || selected.FileVersions[0].ID != version.ID || selected.FileHistoryCount != 1 {
		t.Fatal("selected inspection lost the revalidated version list needed for comparison/replanning")
	}
	sample, err := i.SampleCSV(context.Background(), selected.Assets[0], 10)
	if err != nil || len(sample.Rows) != 1 || sample.Rows[0]["count"] != "7" {
		t.Fatalf("selected historical contract was not usable by normal acquisition: %+v %v", sample, err)
	}
}

func TestInspectHistoricalFileRejectsUnrecognizedHistoryRows(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/catalog/123/fileData.json":
			fmt.Fprint(w, `{"name":"latest","encodingFormat":"CSV"}`)
		case "/data/123/fileData.do":
			fmt.Fprint(w, `<button onclick="fileDetailObj.fn_fileDataDown('123','uddi:11111111-1111-1111-1111-111111111111','','1','1')">latest</button>`)
		case "/tcs/dss/selectHistAndCsvData.do":
			fmt.Fprint(w, `<div id="tab-layer-file-05"><h3>주기성 과거 데이터(<span>1</span>)</h3><table><tbody><tr><td><a class="newSelector">past edition</a></td></tr></tbody></table></div>`)
		default:
			t.Errorf("history drift reached acquisition: %s", r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer s.Close()
	i := dataset.NewInspector(fetch.New(fetch.WithDelay(0)), s.URL)
	if got, err := i.InspectHistory(context.Background(), dataset.Ref{PK: "123", Delivery: "FILE"}, ""); err == nil || got != nil {
		t.Fatal("unrecognized past editions were reported as an empty history")
	}
}

func TestInspectHistoricalFileEnforcesAttachmentIdentityAndBounds(t *testing.T) {
	for _, scenario := range []string{"unlisted", "changed-attachment", "wrong-dataset", "invalid-attachment", "duplicate-asset-name", "conflicting-button-format", "missing-popup", "wrong-filename", "too-many-assets", "duplicate-version", "invalid-version", "too-large-history"} {
		t.Run(scenario, func(t *testing.T) {
			const version = "uddi:22222222-2222-2222-2222-222222222222/2"
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				name := map[string]string{"/catalog/123/fileData.json": "current.json", "/data/123/fileData.do": "current.html", "/tcs/dss/selectHistAndCsvData.do": "list.html", "/tcs/dss/selectDpkDetailInfo.do": "past.html", "/tcs/dss/selectFileDataDownload.do": "resolved.json"}[r.URL.Path]
				if name == "" {
					t.Errorf("rejected contract downloaded a file: %s", r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				b, err := os.ReadFile("testdata/file-history/" + name)
				if err != nil {
					t.Error(err)
				}
				body := string(b)
				switch {
				case name == "list.html" && scenario == "duplicate-version":
					body = strings.ReplaceAll(strings.ReplaceAll(body, "33333333-3333-3333-3333-333333333333", "22222222-2222-2222-2222-222222222222"), `data-public-detail-sn="1"`, `data-public-detail-sn="2"`)
				case name == "list.html" && scenario == "invalid-version":
					body = strings.ReplaceAll(body, "uddi:22222222-2222-2222-2222-222222222222", "https://example.invalid")
				case name == "list.html" && scenario == "too-large-history":
					body = strings.Repeat("x", (8<<20)+1)
				case name == "past.html" && scenario == "wrong-dataset":
					body = strings.ReplaceAll(body, "'123'", "'999'")
				case name == "past.html" && scenario == "invalid-attachment":
					body = strings.ReplaceAll(body, "FILE_PAST", "FILE_../../other")
				case name == "past.html" && scenario == "missing-popup":
					body = "<html>contract unavailable</html>"
				case name == "past.html" && scenario == "conflicting-button-format":
					body = strings.Replace(body, "</section>", `<a onclick="fn_fileDataDown('123','uddi:22222222-2222-2222-2222-222222222222','FILE_PAST','1','json')">JSON</a></section>`, 1)
				case name == "past.html" && (scenario == "duplicate-asset-name" || scenario == "too-many-assets"):
					n := 2
					if scenario == "too-many-assets" {
						n = 9
					}
					for j := 2; j <= n; j++ {
						body = strings.Replace(body, "</section>", fmt.Sprintf(`<a onclick="fn_fileDataDown('123','uddi:22222222-2222-2222-2222-222222222222','FILE_PAST','%d','csv')">CSV</a></section>`, j), 1)
					}
				case name == "resolved.json" && scenario == "changed-attachment":
					body = strings.ReplaceAll(body, "FILE_PAST", "FILE_OTHER")
				case name == "resolved.json" && scenario == "invalid-attachment":
					body = strings.ReplaceAll(body, "FILE_PAST", "FILE_../../other")
				case name == "resolved.json" && scenario == "wrong-filename":
					body = strings.ReplaceAll(body, "snapshot.csv", "snapshot.json")
				case name == "resolved.json" && scenario == "duplicate-asset-name":
					_ = r.ParseForm()
					body = strings.ReplaceAll(body, `"fileDetailSn":"1"`, `"fileDetailSn":"`+r.Form.Get("fileDetailSn")+`"`)
				}
				fmt.Fprint(w, body)
			}))
			defer s.Close()
			requested := version
			if scenario == "unlisted" {
				requested += "0"
			}
			got, err := dataset.NewInspector(fetch.New(fetch.WithDelay(0)), s.URL).InspectHistory(context.Background(), dataset.Ref{PK: "123", Delivery: "FILE"}, requested)
			if err == nil || got != nil {
				t.Fatalf("invalid historical contract accepted: %+v", got)
			}
		})
	}
}

func TestInspectHistoricalFileReportsBoundedListingAndFreshMembership(t *testing.T) {
	for _, count := range []int{0, 33} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			var listedCount atomic.Int64
			listedCount.Store(int64(count))
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/catalog/123/fileData.json":
					fmt.Fprint(w, `{"name":"current","encodingFormat":"CSV"}`)
				case "/data/123/fileData.do":
					fmt.Fprint(w, `<button onclick="fileDetailObj.fn_fileDataDown('123','uddi:11111111-1111-1111-1111-111111111111','','1','1')">latest</button>`)
				case "/tcs/dss/selectHistAndCsvData.do":
					n := int(listedCount.Load())
					fmt.Fprintf(w, `<div id="tab-layer-file-05"><h3>주기성 과거 데이터(<span>%d</span>)</h3><table><tbody>`, n)
					for j := 1; j <= n; j++ {
						fmt.Fprintf(w, `<tr><td><a class="openFileDetailPopup" data-public-pk="uddi:22222222-2222-2222-2222-222222222222" data-public-detail-sn="%d">edition %d</a></td><td>2026-01-01</td></tr>`, j, j)
					}
					fmt.Fprint(w, `</tbody></table></div>`)
				default:
					t.Errorf("unlisted or out-of-bound version reached resolution: %s", r.URL.Path)
					http.Error(w, "unexpected request", 500)
				}
			}))
			defer s.Close()
			i := dataset.NewInspector(fetch.New(fetch.WithDelay(0)), s.URL)
			ref := dataset.Ref{PK: "123", Delivery: "FILE"}
			list, err := i.InspectHistory(context.Background(), ref, "")
			if err != nil || list.FileHistoryCount != count || len(list.FileVersions) != min(count, 32) || list.FileHistoryTruncated != (count > 32) || len(list.Assets) != 0 {
				t.Fatalf("history bounds not exposed: %+v %v", list, err)
			}
			if count != 0 {
				if _, err := i.InspectHistory(context.Background(), ref, "uddi:22222222-2222-2222-2222-222222222222/33"); err == nil {
					t.Fatal("selection exceeded the listed prefix")
				}
				listedCount.Store(0) // The portal removes a formerly advertised edition.
				if _, err := i.InspectHistory(context.Background(), ref, list.FileVersions[0].ID); err == nil {
					t.Fatal("selection did not revalidate current membership")
				}
			}
		})
	}
}
