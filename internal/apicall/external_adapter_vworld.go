package apicall

import (
	"net/url"
	"regexp"
)

// vWorldAdapter demonstrates a provider with several versioned guide families.
// The catalogue still publishes both HTTP and HTTPS detail links, so matching a
// legacy HTTP link is allowed; returned documentation/application addresses are
// fixed HTTPS URLs and the untrusted target itself is never fetched here.
type vWorldAdapter struct{}

var vWorldServiceID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
var vWorldAPINumber = regexp.MustCompile(`^[0-9]+$`)

// vWorldDataCodes binds each reviewed data-guide service ID to the fixed data
// identifier published on that exact guide. Unknown service IDs remain
// discoverable contracts but are not callable until this evidence is added.
var vWorldDataCodes = map[string]string{
	"cadastral": "LP_PA_CBND_BUBUN",
	"adsigg":    "LT_C_ADSIGG_INFO",
	"utiscctv":  "LT_P_UTISCCTV",
	"lhblpn":    "LT_C_LHBLPN",
	"ademd":     "LT_C_ADEMD_INFO",
	"adsido":    "LT_C_ADSIDO_INFO",
	"upisuq151": "LT_C_UPISUQ151",
	"sgisgolf":  "LT_P_SGISGOLF",
	"spbd":      "LT_C_SPBD",
}

var vWorldStaticGuidePaths = map[string]bool{
	"/dev/v4dv_geocoderguide2_s001.do":   true,
	"/dev/v4dv_opn2dmap2guide_s001.do":   true,
	"/dev/v4dv_opnws3dmap3guide_s001.do": true,
	"/dev/v4dv_search2_s001.do":          true,
	"/dev/v4dv_static2_s001.do":          true,
	"/dev/v4dv_wmsguide2_s001.do":        true,
	"/dev/v4dv_wmtsguide_s001.do":        true,
}

var vWorldGuideVersions = map[string]string{
	"/dev/v4dv_2ddataguide2_s002.do":     "2.0",
	"/dev/v4dv_geocoderguide2_s001.do":   "2.0",
	"/dev/v4dv_opn2dmap2guide_s001.do":   "2.0",
	"/dev/v4dv_opnws3dmap3guide_s001.do": "3.0",
	"/dev/v4dv_search2_s001.do":          "2.0",
	"/dev/v4dv_static2_s001.do":          "2.0",
	"/dev/v4dv_wmsguide2_s001.do":        "2.0",
}

func (vWorldAdapter) ContractFor(target *url.URL) (*ExternalContract, bool) {
	host, path, ok := providerURLShape(target)
	if !ok || host != "www.vworld.kr" {
		return nil, false
	}

	serviceID := ""
	family := ""
	switch path {
	case "/dev/v4dv_2ddataguide2_s002.do":
		query, valid := providerQuery(target, []string{"svcIde"}, map[string]bool{"svcIde": true})
		if !valid || !vWorldServiceID.MatchString(query.Get("svcIde")) {
			return nil, false
		}
		serviceID = query.Get("svcIde")
		family = "data"
	case "/dtna/dtna_apiSvcList_s001.do":
		if _, valid := providerQuery(target, []string{"searchKeyword"}, map[string]bool{"searchKeyword": true}); !valid {
			return nil, false
		}
		family = "catalog"
	case "/dtna/dtna_apiSvcFc_s001.do":
		query, valid := providerQuery(target, []string{"apiNum"}, map[string]bool{"apiNum": true})
		if !valid || !vWorldAPINumber.MatchString(query.Get("apiNum")) {
			return nil, false
		}
		serviceID = query.Get("apiNum")
		family = "catalog"
	default:
		if !vWorldStaticGuidePaths[path] || target.RawQuery != "" {
			return nil, false
		}
		switch path {
		case "/dev/v4dv_geocoderguide2_s001.do":
			family = "address"
		case "/dev/v4dv_search2_s001.do":
			family = "search"
		case "/dev/v4dv_wmsguide2_s001.do":
			family = "ogc"
		default:
			family = "unimplemented"
		}
	}
	state := InvocationImplemented
	operations := vWorldOperations(family, serviceID)
	if len(operations) == 0 {
		state = InvocationNotImplemented
	}

	return &ExternalContract{
		Provider:             "VWorld",
		ProviderFamily:       family,
		ProviderServiceID:    serviceID,
		DocumentationURL:     "https://www.vworld.kr/dev/v4apiRefer.do",
		DocumentationVersion: vWorldGuideVersions[path],
		ApplicationURL:       "https://www.vworld.kr/mypo/mypo_apiKey_i001.do",
		AccessMode:           "provider_account_required",
		Auth: &ExternalAuthContract{
			Type:            "api_key",
			Placement:       "query",
			Name:            "key",
			CredentialScope: "https://api.vworld.kr/req/",
		},
		InvocationState: state,
		Operations:      operations,
		VerifiedAt:      "2026-09-01",
	}, true
}

type vWorldOperationSpec struct {
	operation ExternalOperation
	path      string
	service   string
	request   string
	enums     map[string]map[string]bool
}

var vWorldOperationRegistry = map[string][]vWorldOperationSpec{
	"data": {
		vWorldDataOperation("GetFeature", "2D 데이터 객체 조회"),
	},
	"address": {{
		operation: ExternalOperation{Name: "GetCoord", Description: "주소를 좌표로 변환", ResponseKind: "json_or_xml", Params: []Param{
			{Name: "format", Required: "선택", Desc: "json|xml"}, {Name: "errorFormat", Required: "선택", Desc: "json|xml"},
			{Name: "type", Required: "필수", Desc: "road|parcel"}, {Name: "address", Required: "필수", Desc: "검색 주소"},
			{Name: "refine", Required: "선택", Desc: "true|false"}, {Name: "simple", Required: "선택", Desc: "true|false"},
			{Name: "crs", Required: "선택", Desc: "기본 EPSG:4326"},
		}},
		path: "/req/address", service: "address", request: "GetCoord",
		enums: map[string]map[string]bool{
			"format": stringSet("json", "xml"), "errorFormat": stringSet("json", "xml"),
			"type": stringSet("road", "parcel"), "refine": stringSet("true", "false"), "simple": stringSet("true", "false"),
		},
	}},
	"search": {{
		operation: ExternalOperation{Name: "Search", Description: "장소·주소·행정구역·도로 검색", ResponseKind: "json_or_xml", Params: []Param{
			{Name: "format", Required: "선택", Desc: "json|xml"}, {Name: "errorFormat", Required: "선택", Desc: "json|xml"},
			{Name: "size", Required: "선택", Desc: "1~1000"}, {Name: "page", Required: "선택", Desc: "1 이상"},
			{Name: "query", Required: "필수", Desc: "검색어"}, {Name: "type", Required: "필수", Desc: "place|address|district|road"},
			{Name: "category", Required: "조건부", Desc: "address·district에는 필수"}, {Name: "bbox", Required: "선택"},
			{Name: "crs", Required: "선택"},
		}},
		path: "/req/search", service: "search", request: "search",
		enums: map[string]map[string]bool{
			"format": stringSet("json", "xml"), "errorFormat": stringSet("json", "xml"),
			"type": stringSet("place", "address", "district", "road"),
		},
	}},
	"ogc": {
		{
			operation: ExternalOperation{Name: "wms.GetMap", Description: "WMS 지도 이미지", ResponseKind: "image", Params: []Param{
				{Name: "version", Required: "선택"}, {Name: "format", Required: "선택"}, {Name: "exceptions", Required: "선택"},
				{Name: "layers", Required: "필수"}, {Name: "styles", Required: "선택"}, {Name: "bbox", Required: "필수"},
				{Name: "width", Required: "필수", Desc: "1~2048"}, {Name: "height", Required: "필수", Desc: "1~2048"},
				{Name: "transparent", Required: "선택", Desc: "true|false"}, {Name: "bgcolor", Required: "선택"}, {Name: "crs", Required: "선택"},
			}},
			path: "/req/wms", service: "WMS", request: "GetMap",
			enums: map[string]map[string]bool{"transparent": stringSet("TRUE", "FALSE", "true", "false")},
		},
		{
			operation: ExternalOperation{Name: "wms.GetCapabilities", Description: "WMS capability 문서", ResponseKind: "xml", Params: []Param{
				{Name: "version", Required: "선택"}, {Name: "format", Required: "선택"}, {Name: "exceptions", Required: "선택"},
			}},
			path: "/req/wms", service: "WMS", request: "GetCapabilities",
		},
		{
			operation: ExternalOperation{Name: "wms.GetFeatureInfo", Description: "WMS 화면 좌표의 feature 정보", ResponseKind: "json_or_xml", Params: []Param{
				{Name: "version", Required: "선택"}, {Name: "layers", Required: "필수"}, {Name: "styles", Required: "선택"},
				{Name: "crs", Required: "선택"}, {Name: "bbox", Required: "필수"}, {Name: "width", Required: "필수"},
				{Name: "height", Required: "필수"}, {Name: "query_layers", Required: "필수"}, {Name: "info_format", Required: "선택"},
				{Name: "feature_count", Required: "선택", Desc: "1~1000"}, {Name: "i", Required: "필수", Desc: "0 이상"},
				{Name: "j", Required: "필수", Desc: "0 이상"}, {Name: "exceptions", Required: "선택"},
			}},
			path: "/req/wms", service: "WMS", request: "GetFeatureInfo",
		},
		{
			operation: ExternalOperation{Name: "wfs.GetFeature", Description: "WFS feature 조회", ResponseKind: "xml", Params: []Param{
				{Name: "version", Required: "선택"}, {Name: "output", Required: "선택"}, {Name: "format_options", Required: "선택"},
				{Name: "exceptions", Required: "선택"}, {Name: "typename", Required: "필수"}, {Name: "featureid", Required: "선택"},
				{Name: "bbox", Required: "선택"}, {Name: "propertyname", Required: "선택"}, {Name: "maxfeatures", Required: "선택", Desc: "1~1000"},
				{Name: "count", Required: "선택", Desc: "1~1000"}, {Name: "startindex", Required: "선택", Desc: "0 이상"},
				{Name: "sortby", Required: "선택"}, {Name: "srsname", Required: "선택"}, {Name: "filter", Required: "선택"},
			}},
			path: "/req/wfs", service: "WFS", request: "GetFeature",
		},
		{
			operation: ExternalOperation{Name: "wfs.GetCapabilities", Description: "WFS capability 문서", ResponseKind: "xml", Params: []Param{
				{Name: "version", Required: "선택"}, {Name: "output", Required: "선택"}, {Name: "exceptions", Required: "선택"},
			}},
			path: "/req/wfs", service: "WFS", request: "GetCapabilities",
		},
	},
}

func vWorldDataOperation(name, description string) vWorldOperationSpec {
	return vWorldOperationSpec{
		operation: ExternalOperation{Name: name, Description: description, ResponseKind: "json_or_xml", Params: []Param{
			{Name: "format", Required: "선택", Desc: "json|xml"}, {Name: "errorFormat", Required: "선택", Desc: "json|xml"},
			{Name: "size", Required: "선택", Desc: "1~1000"}, {Name: "page", Required: "선택", Desc: "1 이상"},
			{Name: "geomFilter", Required: "조건부", Desc: "POINT|LINESTRING|POLYGON|MULTIPOLYGON|BOX"},
			{Name: "attrFilter", Required: "조건부", Desc: "속성명:연산자:비교값"}, {Name: "columns", Required: "선택"},
			{Name: "geometry", Required: "선택"}, {Name: "attribute", Required: "선택"}, {Name: "buffer", Required: "선택"},
			{Name: "crs", Required: "선택"},
		}},
		path: "/req/data", service: "data", request: name,
		enums: map[string]map[string]bool{"format": stringSet("json", "xml"), "errorFormat": stringSet("json", "xml")},
	}
}

func vWorldOperationFor(family, name string) (vWorldOperationSpec, bool) {
	for _, spec := range vWorldOperationRegistry[family] {
		if spec.operation.Name == name {
			return spec, true
		}
	}
	return vWorldOperationSpec{}, false
}

func vWorldOperations(family, serviceID string) []ExternalOperation {
	if family == "data" {
		if _, ok := vWorldDataCodes[serviceID]; !ok {
			return nil
		}
	}
	specs := vWorldOperationRegistry[family]
	operations := make([]ExternalOperation, 0, len(specs))
	for _, spec := range specs {
		operation := spec.operation
		operation.Params = append([]Param(nil), operation.Params...)
		operations = append(operations, operation)
	}
	return operations
}
