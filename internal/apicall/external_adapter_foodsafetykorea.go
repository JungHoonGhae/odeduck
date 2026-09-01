package apicall

import (
	"net/url"
	"regexp"
)

// foodSafetyKoreaAdapter demonstrates a service-specific detail link whose API
// key and service identifier are path segments in the eventual request URL.
type foodSafetyKoreaAdapter struct{}

const foodSafetyKoreaListOperation = "list"

var foodSafetyKoreaBaseParams = []Param{
	{Name: "dataType", Required: "필수", Desc: "json|xml"},
	{Name: "startIdx", Required: "필수", Desc: "1 이상의 시작 위치"},
	{Name: "endIdx", Required: "필수", Desc: "종료 위치; 한 요청 최대 1,000건"},
}

var foodSafetyKoreaServiceID = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

var foodSafetyKoreaDetailQuery = map[string]bool{
	"menu_grp":    true,
	"menu_no":     true,
	"show_cnt":    true,
	"start_idx":   true,
	"svc_no":      true,
	"svc_type_cd": true,
}

func (foodSafetyKoreaAdapter) ContractFor(target *url.URL) (*ExternalContract, bool) {
	host, path, ok := providerURLShape(target)
	if !ok || host != "www.foodsafetykorea.go.kr" ||
		path != "/api/openApiInfo.do" {
		return nil, false
	}
	query, ok := providerQuery(target, []string{"svc_no"}, foodSafetyKoreaDetailQuery)
	if !ok || !foodSafetyKoreaServiceID.MatchString(query.Get("svc_no")) {
		return nil, false
	}
	documentationTarget := *target
	documentationTarget.Scheme = "https"
	documentationTarget.Host = "www.foodsafetykorea.go.kr"
	return &ExternalContract{
		Provider:          "FoodSafetyKorea",
		ProviderFamily:    "dataset",
		ProviderServiceID: query.Get("svc_no"),
		DocumentationURL:  documentationTarget.String(),
		ApplicationURL:    "https://www.foodsafetykorea.go.kr/api/newUserApiKey.do?menu_grp=MENU_GRP32&menu_no=691",
		AccessMode:        "provider_account_required",
		Auth: &ExternalAuthContract{
			Type:            "api_key",
			Placement:       "path",
			Name:            "keyId",
			CredentialScope: "https://openapi.foodsafetykorea.go.kr/api/",
		},
		InvocationState: InvocationImplemented,
		Operations: []ExternalOperation{{
			Name: foodSafetyKoreaListOperation, Description: "서비스별 식품안전 데이터 목록", ResponseKind: "json_or_xml", DynamicParams: true,
			Params: append([]Param(nil), foodSafetyKoreaBaseParams...),
		}},
		VerifiedAt: "2026-09-01",
	}, true
}
