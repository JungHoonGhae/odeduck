package apicall

import (
	"net/url"
	"testing"
)

func TestExternalProviderAdapterRegistry(t *testing.T) {
	want := []struct {
		id, provider string
		revision     int
		canaries     int
	}{
		{"safetykorea", "SafetyKorea", 2, 1},
		{"vworld", "VWorld", 3, 5},
		{"foodsafetykorea", "FoodSafetyKorea", 2, 1},
		{"seoul-open-data", "Seoul Open Data Plaza", 2, 4},
	}
	got := ExternalProviderAdapters()
	if len(got) != len(want) {
		t.Fatalf("registered adapters = %d, want %d: %+v", len(got), len(want), got)
	}
	seen := map[string]bool{}
	for i := range want {
		if got[i].ID != want[i].id || got[i].Revision != want[i].revision ||
			got[i].Provider != want[i].provider || len(got[i].Canaries) != want[i].canaries {
			t.Errorf("adapter[%d] = %+v, want id=%s revision=%d provider=%s canaries=%d",
				i, got[i], want[i].id, want[i].revision, want[i].provider, want[i].canaries)
		}
		if seen[got[i].ID] {
			t.Errorf("duplicate adapter ID %q", got[i].ID)
		}
		seen[got[i].ID] = true
		for _, canary := range got[i].Canaries {
			contract := contractFromURL(t, canary.URL)
			if contract == nil || contract.AdapterID != got[i].ID || contract.AdapterRevision != got[i].Revision ||
				contract.Provider != got[i].Provider {
				t.Errorf("canary %q contract = %+v, want %s@r%d/%s",
					canary.URL, contract, got[i].ID, got[i].Revision, got[i].Provider)
			}
		}
	}

	// The returned inventory is a snapshot, not a mutable view of the registry.
	got[0].ID = "changed"
	got[0].Canaries[0].PK = "changed"
	if ExternalProviderAdapters()[0].ID != "safetykorea" {
		t.Fatal("caller mutated the adapter registry through the inventory")
	}
	if ExternalProviderAdapters()[0].Canaries[0].PK != "15116894" {
		t.Fatal("caller mutated a nested canary through the inventory")
	}
}

func TestExternalProviderAdaptersRecognizeObservedLinkFamilies(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		adapterID string
		provider  string
		serviceID string
		placement string
		authName  string
		scope     string
	}{
		{"SafetyKorea HTTPS hub", "https://www.safetykorea.kr/release/openapi", "safetykorea", "SafetyKorea", "", "header", "AuthKey", "https://www.safetykorea.kr/openapi/api/"},
		{"VWorld legacy 2D guide", "http://www.vworld.kr/dev/v4dv_2ddataguide2_s002.do?svcIde=cadastral", "vworld", "VWorld", "cadastral", "query", "key", "https://api.vworld.kr/req/"},
		{"VWorld HTTPS static guide", "https://www.vworld.kr/dev/v4dv_wmsguide2_s001.do", "vworld", "VWorld", "", "query", "key", "https://api.vworld.kr/req/"},
		{"VWorld data service list", "https://www.vworld.kr/dtna/dtna_apiSvcFc_s001.do?apiNum=128", "vworld", "VWorld", "128", "query", "key", "https://api.vworld.kr/req/"},
		{"FoodSafetyKorea legacy detail", "http://www.foodsafetykorea.go.kr/api/openApiInfo.do?menu_grp=MENU_GRP31&menu_no=661&show_cnt=10&start_idx=1&svc_no=COOKRCP01", "foodsafetykorea", "FoodSafetyKorea", "COOKRCP01", "path", "keyId", "https://openapi.foodsafetykorea.go.kr/api/"},
		{"FoodSafetyKorea HTTPS detail", "https://www.foodsafetykorea.go.kr/api/openApiInfo.do?svc_no=I-0040&svc_type_cd=API_TYPE06", "foodsafetykorea", "FoodSafetyKorea", "I-0040", "path", "keyId", "https://openapi.foodsafetykorea.go.kr/api/"},
		{"Seoul metro legacy detail", "http://data.seoul.go.kr/dataList/datasetView.do?infId=OA-12764&srvType=A&serviceKind=1&currentPageNo=1", "seoul-open-data", "Seoul Open Data Plaza", "OA-12764", "path", "KEY", "http://swopenapi.seoul.go.kr/api/subway/"},
		{"Seoul metro current detail", "https://data.seoul.go.kr/dataList/OA-15799/A/1/datasetView.do", "seoul-open-data", "Seoul Open Data Plaza", "OA-15799", "path", "KEY", "http://swopenapi.seoul.go.kr/api/subway/"},
		{"Seoul general current detail", "https://data.seoul.go.kr/dataList/OA-15442/S/1/datasetView.do", "seoul-open-data", "Seoul Open Data Plaza", "OA-15442", "path", "KEY", "http://openapi.seoul.go.kr:8088/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract := contractFromURL(t, tt.raw)
			if contract == nil {
				t.Fatal("known provider URL did not match an adapter")
			}
			wantRevision := 2
			if tt.adapterID == "vworld" {
				wantRevision = 3
			}
			if contract.AdapterID != tt.adapterID || contract.AdapterRevision != wantRevision || contract.Provider != tt.provider ||
				contract.ProviderServiceID != tt.serviceID || contract.Auth == nil ||
				contract.Auth.Placement != tt.placement || contract.Auth.Name != tt.authName || contract.Auth.CredentialScope != tt.scope ||
				contract.DocumentationURL == "" || contract.ApplicationURL == "" ||
				contract.InvocationState == "" || contract.VerifiedAt == "" {
				t.Fatalf("contract = %+v, auth=%+v", contract, contract.Auth)
			}
		})
	}
}

func TestExternalProviderContractsExposeOnlyImplementedTypedOperations(t *testing.T) {
	tests := []struct {
		raw, family, state string
		operations         []string
	}{
		{"https://www.safetykorea.kr/release/openapi", "product-safety", InvocationImplemented,
			[]string{"certificationList", "certificationDetail", "recallList", "recallDetail", "fRecallList"}},
		{"https://www.foodsafetykorea.go.kr/api/openApiInfo.do?svc_no=I-0040", "dataset", InvocationImplemented, []string{"list"}},
		{"https://www.vworld.kr/dev/v4dv_2ddataguide2_s002.do?svcIde=cadastral", "data", InvocationImplemented, []string{"GetFeature"}},
		{"https://www.vworld.kr/dev/v4dv_2ddataguide2_s002.do?svcIde=unreviewed", "data", InvocationNotImplemented, nil},
		{"https://www.vworld.kr/dev/v4dv_geocoderguide2_s001.do", "address", InvocationImplemented, []string{"GetCoord"}},
		{"https://www.vworld.kr/dev/v4dv_search2_s001.do", "search", InvocationImplemented, []string{"Search"}},
		{"https://www.vworld.kr/dev/v4dv_wmsguide2_s001.do", "ogc", InvocationImplemented,
			[]string{"wms.GetMap", "wms.GetCapabilities", "wms.GetFeatureInfo", "wfs.GetFeature", "wfs.GetCapabilities"}},
		{"https://www.vworld.kr/dtna/dtna_apiSvcList_s001.do?searchKeyword=%EA%B0%9C%EB%B3%84%EA%B3%B5%EC%8B%9C%EC%A7%80%EA%B0%80", "catalog", InvocationNotImplemented, nil},
		{"https://data.seoul.go.kr/dataList/OA-15799/A/1/datasetView.do", "metro", InvocationBlockedInsecureTransport, nil},
	}
	for _, tt := range tests {
		t.Run(tt.family, func(t *testing.T) {
			contract := contractFromURL(t, tt.raw)
			if contract == nil {
				t.Fatal("contract missing")
			}
			if contract.ProviderFamily != tt.family || contract.InvocationState != tt.state {
				t.Fatalf("contract family/state = %q/%q", contract.ProviderFamily, contract.InvocationState)
			}
			if len(contract.Operations) != len(tt.operations) {
				t.Fatalf("operations = %#v, want %v", contract.Operations, tt.operations)
			}
			for i, want := range tt.operations {
				if contract.Operations[i].Name != want || len(contract.Operations[i].Params) == 0 && want != "wms.GetCapabilities" && want != "wfs.GetCapabilities" {
					t.Errorf("operation[%d] = %#v, want %s with params", i, contract.Operations[i], want)
				}
			}
		})
	}
}

func TestVWorldContractSurfacesEverySupportedOptionalParameter(t *testing.T) {
	tests := []struct {
		raw, operation string
		want           []string
	}{
		{
			"https://www.vworld.kr/dev/v4dv_2ddataguide2_s002.do?svcIde=cadastral",
			"GetFeature",
			[]string{"geomFilter", "attrFilter", "format", "errorFormat", "size", "page", "columns", "geometry", "attribute", "buffer", "crs"},
		},
		{
			"https://www.vworld.kr/dev/v4dv_wmsguide2_s001.do",
			"wms.GetMap",
			[]string{"version", "format", "exceptions", "layers", "styles", "bbox", "width", "height", "transparent", "bgcolor", "crs"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.operation, func(t *testing.T) {
			contract := contractFromURL(t, tt.raw)
			var got *ExternalOperation
			for i := range contract.Operations {
				if contract.Operations[i].Name == tt.operation {
					got = &contract.Operations[i]
					break
				}
			}
			if got == nil {
				t.Fatalf("operation %s not found", tt.operation)
			}
			names := map[string]bool{}
			for _, param := range got.Params {
				names[param.Name] = true
			}
			for _, name := range tt.want {
				if !names[name] {
					t.Errorf("%s accepts %s but describe contract does not surface it", tt.operation, name)
				}
			}
		})
	}
}

func TestExternalHandoffNextActionMatchesInvocationCapability(t *testing.T) {
	tests := []struct {
		name, raw, want string
	}{
		{"implemented", "https://www.safetykorea.kr/release/openapi", HandoffRequestAccess},
		{"not implemented", "https://www.vworld.kr/dtna/dtna_apiSvcList_s001.do?searchKeyword=cadastral", HandoffUseProviderDirectly},
		{"blocked insecure transport", "https://data.seoul.go.kr/dataList/OA-15799/A/1/datasetView.do", HandoffChooseAnother},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var spec APISpec
			if err := spec.setExternalHandoff(tt.raw); err != nil {
				t.Fatal(err)
			}
			if spec.Handoff == nil || spec.Handoff.NextAction != tt.want {
				t.Fatalf("handoff = %+v, want nextAction=%s", spec.Handoff, tt.want)
			}
		})
	}
}

func TestExternalProviderAdaptersFailClosedOnLookalikes(t *testing.T) {
	tests := []string{
		"https://www.safetykorea.kr/release/openapi?next=other",
		"https://safetykorea.kr/release/openapi",
		"http://www.safetykorea.kr/release/openapi",
		"https://www.vworld.kr/dev/unverified.do",
		"https://www.vworld.kr/dev/v4dv_wmsguide2_s001.do?next=other",
		"https://www.vworld.kr/dev/v4dv_2ddataguide2_s002.do",
		"https://www.vworld.kr/dev/v4dv_2ddataguide2_s002.do?svcIde=a&svcIde=b",
		"https://www.vworld.kr/dev/v4dv_2ddataguide2_s002.do?svcIde=%ED%95%9C%EA%B8%80",
		"https://www.vworld.kr:8443/dev/v4dv_wmsguide2_s001.do",
		"https://vworld.kr.evil.example/dev/v4dv_wmsguide2_s001.do",
		"https://vworld.kr/dev/v4dv_wmsguide2_s001.do",
		"https://www.foodsafetykorea.go.kr/api/openApiInfo.do",
		"https://www.foodsafetykorea.go.kr/api/openApiInfo.do?svc_no=C005&delete=true",
		"https://www.foodsafetykorea.go.kr/api/openApiInfo.do?svc_no=%ED%95%9C%EA%B8%80",
		"https://foodsafetykorea.go.kr/api/openApiInfo.do?svc_no=C005",
		"https://www.foodsafetykorea.go.kr/api/openApiInfo%2Edo?svc_no=C005",
		"https://data.seoul.go.kr/dataList/OA-15799/X/1/datasetView.do",
		"https://data.seoul.go.kr/dataList/OA-15799/A/1/datasetView.do?next=other",
		"https://data.seoul.go.kr/dataList/datasetView.do?infId=OA-1&srvType=X",
		"https://data.seoul.go.kr/dataList/datasetView.do?infId=OA-12764&srvType=A&serviceKind=2",
		"https://data.seoul.go.kr/dataList/datasetView.do?infId=OA-12764&srvType=A&serviceKind=1&currentPageNo=0",
		"https://data.seoul.go.kr/dataList/OA-99999/A/1/datasetView.do",
		"https://" + "data.seoul.go.kr@" + "evil.example/dataList/OA-15799/A/1/datasetView.do",
		"https://data.seoul.go.kr/dataList/OA-15799/A/1/datasetView.do#other",
	}
	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			if contract := contractFromURL(t, raw); contract != nil {
				t.Fatalf("lookalike received contract: %+v", contract)
			}
		})
	}
}

func TestOnlyOneExternalProviderAdapterMatchesEachCanary(t *testing.T) {
	for _, info := range ExternalProviderAdapters() {
		for _, canary := range info.Canaries {
			target, err := url.Parse(canary.URL)
			if err != nil {
				t.Fatal(err)
			}
			matches := 0
			for _, registered := range externalProviderAdapters {
				if _, ok := registered.adapter.ContractFor(target); ok {
					matches++
				}
			}
			if matches != 1 {
				t.Errorf("adapter canary %s/%s matched %d adapters, want exactly one", info.ID, canary.Variant, matches)
			}
		}
	}
}

func contractFromURL(t *testing.T, raw string) *ExternalContract {
	t.Helper()
	target, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	return knownExternalContract(target)
}
