package dataset

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/oddsock/internal/catalog"
	"github.com/JungHoonGhae/oddsock/internal/fetch"
)

func TestUnifiedInspectorOwnsFileDispatchAndAssetObservation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	const pk = "15151047"
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: []catalog.Entry{{
		PK: pk, Title: "성장상권", SvcType: catalog.SvcFILE, DataTypes: []string{"FILE"},
	}}}).Save(); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/catalog/" + pk + "/fileData.json":
			_, _ = w.Write([]byte(`{"alternateName":"성장상권","creator":{"name":"소상공인시장진흥공단"},"encodingFormat":"CSV"}`))
		case "/data/" + pk + "/fileData.do":
			_, _ = w.Write([]byte(`<button onclick="fileDetailObj.fn_fileDataDown('15151047','uddi:growth','','1','3')">다운로드</button>`))
		case "/tcs/dss/selectFileDataDownload.do":
			_, _ = w.Write([]byte(`{"status":true,"atchFileId":"FILE_123","fileDetailSn":"1","fileDataRegistVO":{"dataNm":"성장상권","orginlFileNm":"growth.csv","atchFileExtsn":"csv"}}`))
		case "/cmm/cmm/fileDownload.do":
			_, _ = w.Write([]byte("CRTR_YM,MJR_BZZNNO\n202401,100\n"))
		default:
			t.Fatalf("unexpected request: %s", r.URL)
		}
	}))
	defer srv.Close()

	result, err := NewUnifiedInspector(fetch.New(fetch.WithDelay(0)), srv.URL).Inspect(context.Background(), InspectionRequest{
		PK: pk, Observe: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Delivery != catalog.SvcFILE || result.File == nil || result.Observation == nil ||
		len(result.Observation.Files) != 1 || strings.Join(result.Observation.Files[0].Columns, ",") != "CRTR_YM,MJR_BZZNNO" {
		t.Fatalf("unified result = %+v", result)
	}
}

func TestUnifiedInspectorReturnsBothContractsForAPIDatasetWithFileRepresentation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	const pk = "15151048"
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: []catalog.Entry{{
		PK: pk, Title: "복수 제공 데이터", SvcType: catalog.SvcREST, DataTypes: []string{"API", "FILE"},
		OfficialAPI: &catalog.OfficialAPIContract{APIType: catalog.SvcREST, Operations: []catalog.OfficialAPIOperation{{
			Name: "getBoth", URL: "https://apis.data.go.kr/example/getBoth",
		}}},
	}}}).Save(); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/data/" + pk + "/openapi.do":
			http.Error(w, "fallback unavailable", http.StatusBadGateway)
		case "/catalog/" + pk + "/fileData.json":
			_, _ = w.Write([]byte(`{"alternateName":"복수 제공 데이터","encodingFormat":"CSV"}`))
		case "/data/" + pk + "/fileData.do":
			_, _ = w.Write([]byte(`<button onclick="fileDetailObj.fn_fileDataDown('15151048','uddi:both','','1','3')">다운로드</button>`))
		case "/tcs/dss/selectFileDataDownload.do":
			_, _ = w.Write([]byte(`{"status":true,"atchFileId":"FILE_BOTH","fileDetailSn":"1","fileDataRegistVO":{"dataNm":"복수 제공 데이터","orginlFileNm":"both.csv","atchFileExtsn":"csv"}}`))
		default:
			t.Fatalf("unexpected request: %s", r.URL)
		}
	}))
	defer srv.Close()

	result, err := NewUnifiedInspector(fetch.New(fetch.WithDelay(0)), srv.URL).Inspect(context.Background(), InspectionRequest{PK: pk})
	if err != nil {
		t.Fatal(err)
	}
	if result.Delivery != catalog.SvcREST || strings.Join(result.Deliveries, ",") != "API,FILE" || result.API == nil || result.File == nil {
		t.Fatalf("dual inspection = %+v", result)
	}
}

func TestUnifiedInspectorCanSelectFileRepresentation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	const pk = "15151049"
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: []catalog.Entry{{
		PK: pk, Title: "복수 제공 데이터", SvcType: catalog.SvcLINK, DataTypes: []string{"API", "FILE"},
	}}}).Save(); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/catalog/" + pk + "/fileData.json":
			_, _ = w.Write([]byte(`{"alternateName":"복수 제공 데이터","encodingFormat":"CSV"}`))
		case "/data/" + pk + "/fileData.do":
			_, _ = w.Write([]byte(`<button onclick="fileDetailObj.fn_fileDataDown('15151049','uddi:both','','1','3')">다운로드</button>`))
		case "/tcs/dss/selectFileDataDownload.do":
			_, _ = w.Write([]byte(`{"status":true,"atchFileId":"FILE_BOTH","fileDetailSn":"1","fileDataRegistVO":{"dataNm":"복수 제공 데이터","orginlFileNm":"both.csv","atchFileExtsn":"csv"}}`))
		default:
			t.Fatalf("unexpected request: %s", r.URL)
		}
	}))
	defer srv.Close()

	result, err := NewUnifiedInspector(fetch.New(fetch.WithDelay(0)), srv.URL).Inspect(context.Background(), InspectionRequest{
		PK: pk, Delivery: "file",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Delivery != catalog.SvcFILE || strings.Join(result.Deliveries, ",") != "FILE" || result.API != nil || result.File == nil {
		t.Fatalf("selected FILE inspection = %+v", result)
	}
}

func TestUnifiedInspectorRejectsUnsupportedCatalogDelivery(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	const pk = "300"
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: []catalog.Entry{{
		PK: pk, Title: "표준데이터", SvcType: "", DataTypes: nil,
	}}}).Save(); err != nil {
		t.Fatal(err)
	}
	_, err := NewUnifiedInspector(fetch.New(fetch.WithDelay(0)), "https://data.go.kr").Inspect(context.Background(), InspectionRequest{PK: pk})
	if err == nil || !strings.Contains(err.Error(), "제공형을 자동 검사할 수 없습니다") {
		t.Fatalf("error = %v", err)
	}
}

func TestSelectAssetRequiresAnExactAdvertisedName(t *testing.T) {
	assets := []Asset{{Name: "sales-2025.csv"}, {Name: "sales-2026.csv"}}
	selected, err := selectAsset(assets, "sales-2026.csv")
	if err != nil || selected.Name != "sales-2026.csv" {
		t.Fatalf("selected = %+v, error = %v", selected, err)
	}
	if _, err := selectAsset(assets, "sales-latest.csv"); err == nil || !strings.Contains(err.Error(), "찾지 못했습니다") {
		t.Fatalf("unadvertised asset error = %v", err)
	}
	if _, err := selectAsset(nil, ""); err == nil || !strings.Contains(err.Error(), "자산이 없습니다") {
		t.Fatalf("empty asset list error = %v", err)
	}
}
