package apicall

import (
	"net/url"
	"strings"
)

// externalProviderAdapter is the deliberately small seam for LINK providers.
// Implementations own both recognition and the contract they can prove. They do
// not fetch the publisher URL: it remains untrusted input, and a contract match
// must be possible from the portal-supplied URL alone.
type externalProviderAdapter interface {
	ContractFor(target *url.URL) (*ExternalContract, bool)
}

// ExternalProviderAdapterInfo is the public maintenance inventory for one
// registered adapter. Doctor resolves every canary through data.go.kr and
// requires it to still produce this adapter ID.
type ExternalProviderAdapterInfo struct {
	ID       string                          `json:"id"`
	Revision int                             `json:"revision"`
	Provider string                          `json:"provider"`
	Canaries []ExternalProviderAdapterCanary `json:"canaries"`
}

// ExternalProviderAdapterCanary covers one materially different URL or
// credential variant. URL is the reviewed exact portal result: even a change
// that still matches is surfaced so a maintainer rechecks the provider docs.
type ExternalProviderAdapterCanary struct {
	Variant string `json:"variant"`
	PK      string `json:"pk"`
	URL     string `json:"url"`
}

type registeredExternalProviderAdapter struct {
	info    ExternalProviderAdapterInfo
	adapter externalProviderAdapter
}

var externalProviderAdapters = []registeredExternalProviderAdapter{
	{
		info: ExternalProviderAdapterInfo{
			ID:       "safetykorea",
			Revision: 2,
			Provider: "SafetyKorea",
			Canaries: []ExternalProviderAdapterCanary{
				{Variant: "openapi-hub", PK: "15116894", URL: "https://www.safetykorea.kr/release/openapi"},
			},
		},
		adapter: safetyKoreaAdapter{},
	},
	{
		info: ExternalProviderAdapterInfo{
			ID:       "vworld",
			Revision: 3,
			Provider: "VWorld",
			Canaries: []ExternalProviderAdapterCanary{
				{Variant: "2d-data-legacy", PK: "15056910", URL: "http://www.vworld.kr/dev/v4dv_2ddataguide2_s002.do?svcIde=cadastral"},
				{Variant: "geocoder-guide", PK: "15101106", URL: "https://www.vworld.kr/dev/v4dv_geocoderguide2_s001.do"},
				{Variant: "wms-guide", PK: "15057570", URL: "https://www.vworld.kr/dev/v4dv_wmsguide2_s001.do"},
				{Variant: "search-guide", PK: "15058799", URL: "https://www.vworld.kr/dev/v4dv_search2_s001.do"},
				{Variant: "data-service-list", PK: "15124014", URL: "https://www.vworld.kr/dtna/dtna_apiSvcList_s001.do?searchKeyword=%EA%B0%9C%EB%B3%84%EA%B3%B5%EC%8B%9C%EC%A7%80%EA%B0%80"},
			},
		},
		adapter: vWorldAdapter{},
	},
	{
		info: ExternalProviderAdapterInfo{
			ID:       "foodsafetykorea",
			Revision: 2,
			Provider: "FoodSafetyKorea",
			Canaries: []ExternalProviderAdapterCanary{
				{Variant: "service-detail", PK: "15058359", URL: "https://www.foodsafetykorea.go.kr/api/openApiInfo.do?menu_grp=MENU_GRP31&menu_no=656&show_cnt=10&start_idx=1&svc_no=I-0040&svc_type_cd=API_TYPE06"},
			},
		},
		adapter: foodSafetyKoreaAdapter{},
	},
	{
		info: ExternalProviderAdapterInfo{
			ID:       "seoul-open-data",
			Revision: 2,
			Provider: "Seoul Open Data Plaza",
			Canaries: []ExternalProviderAdapterCanary{
				{Variant: "metro-legacy", PK: "15058052", URL: "http://data.seoul.go.kr/dataList/datasetView.do?infId=OA-12764&srvType=A&serviceKind=1&currentPageNo=1"},
				{Variant: "metro-current", PK: "15125683", URL: "https://data.seoul.go.kr/dataList/OA-15799/A/1/datasetView.do"},
				{Variant: "general-legacy", PK: "15044244", URL: "http://data.seoul.go.kr/dataList/datasetView.do?infId=OA-12033&srvType=S&serviceKind=1&currentPageNo=1"},
				{Variant: "general-current", PK: "15058404", URL: "http://data.seoul.go.kr/dataList/OA-15442/S/1/datasetView.do"},
			},
		},
		adapter: seoulOpenDataAdapter{},
	},
}

// ExternalProviderAdapters returns a copy so diagnostics and documentation can
// enumerate coverage without being able to mutate matching order or canaries.
func ExternalProviderAdapters() []ExternalProviderAdapterInfo {
	infos := make([]ExternalProviderAdapterInfo, len(externalProviderAdapters))
	for i := range externalProviderAdapters {
		infos[i] = externalProviderAdapters[i].info
		infos[i].Canaries = append([]ExternalProviderAdapterCanary(nil), externalProviderAdapters[i].info.Canaries...)
	}
	return infos
}

// knownExternalContract is deliberately a narrow, evidence-backed registry. A
// URL pattern is registered only after official application and authentication
// documents have been inspected. A known host at an unverified path retains the
// generic inspection_required handoff; hostname or URL words are never enough.
func knownExternalContract(target *url.URL) *ExternalContract {
	for _, registered := range externalProviderAdapters {
		contract, ok := registered.adapter.ContractFor(target)
		if !ok {
			continue
		}
		contract.AdapterID = registered.info.ID
		contract.AdapterRevision = registered.info.Revision
		return contract
	}
	return nil
}

// providerURLShape applies the checks shared by every matcher. Individual
// adapters still decide whether HTTP is an observed legacy catalogue link or
// whether HTTPS is mandatory, and which exact path/query shapes are valid.
func providerURLShape(target *url.URL) (host, path string, ok bool) {
	if target == nil || target.User != nil || target.Fragment != "" {
		return "", "", false
	}
	host = strings.TrimSuffix(strings.ToLower(target.Hostname()), ".")
	path = strings.TrimSuffix(target.EscapedPath(), "/")
	switch strings.ToLower(target.Scheme) {
	case "http":
		if target.Port() != "" && target.Port() != "80" {
			return "", "", false
		}
	case "https":
		if target.Port() != "" && target.Port() != "443" {
			return "", "", false
		}
	default:
		return "", "", false
	}
	return host, path, true
}

func isHTTPS(target *url.URL) bool {
	return strings.EqualFold(target.Scheme, "https")
}

// providerQuery accepts only one non-empty value per allowed key. This keeps a
// familiar provider host with an unrelated action/query from inheriting an API
// contract. The returned map is safe for service-ID extraction.
func providerQuery(target *url.URL, required []string, allowed map[string]bool) (url.Values, bool) {
	values, err := url.ParseQuery(target.RawQuery)
	if err != nil {
		return nil, false
	}
	for key, value := range values {
		if !allowed[key] || len(value) != 1 || strings.TrimSpace(value[0]) == "" {
			return nil, false
		}
	}
	for _, key := range required {
		if len(values[key]) != 1 || strings.TrimSpace(values.Get(key)) == "" {
			return nil, false
		}
	}
	return values, true
}
