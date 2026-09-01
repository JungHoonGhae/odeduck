package apicall

import (
	"net/url"
	"regexp"
)

// seoulOpenDataAdapter demonstrates one provider with legacy query-based and
// current path-based dataset detail URLs. Both shapes resolve to the same
// provider-wide key contract.
type seoulOpenDataAdapter struct{}

var seoulDatasetPath = regexp.MustCompile(`^/dataList/(OA-[0-9]+)/(A|S)/1/datasetView\.do$`)
var seoulDatasetID = regexp.MustCompile(`^OA-[0-9]+$`)
var seoulPositivePage = regexp.MustCompile(`^[1-9][0-9]*$`)

var seoulLegacyDetailQuery = map[string]bool{
	"infId":         true,
	"srvType":       true,
	"serviceKind":   true,
	"currentPageNo": true,
}

// These are the six high-demand Seoul LINK services in the measured top-50
// sample. The metro and general keys are issued separately, so an unverified OA
// identifier must stay inspection_required instead of inheriting either scope.
var seoulServiceCredentialScope = map[string]string{
	"OA-12764": "http://swopenapi.seoul.go.kr/api/subway/",
	"OA-12601": "http://swopenapi.seoul.go.kr/api/subway/",
	"OA-15799": "http://swopenapi.seoul.go.kr/api/subway/",
	"OA-15442": "http://openapi.seoul.go.kr:8088/",
	"OA-12033": "http://openapi.seoul.go.kr:8088/",
	"OA-12034": "http://openapi.seoul.go.kr:8088/",
}

func (seoulOpenDataAdapter) ContractFor(target *url.URL) (*ExternalContract, bool) {
	host, path, ok := providerURLShape(target)
	if !ok || host != "data.seoul.go.kr" {
		return nil, false
	}

	serviceID := ""
	if matches := seoulDatasetPath.FindStringSubmatch(path); matches != nil {
		if target.RawQuery != "" {
			return nil, false
		}
		serviceID = matches[1]
	} else if path == "/dataList/datasetView.do" {
		query, valid := providerQuery(target, []string{"infId", "srvType", "serviceKind"}, seoulLegacyDetailQuery)
		if !valid || !seoulDatasetID.MatchString(query.Get("infId")) ||
			(query.Get("srvType") != "A" && query.Get("srvType") != "S") || query.Get("serviceKind") != "1" ||
			(query.Has("currentPageNo") && !seoulPositivePage.MatchString(query.Get("currentPageNo"))) {
			return nil, false
		}
		serviceID = query.Get("infId")
	} else {
		return nil, false
	}
	credentialScope, knownService := seoulServiceCredentialScope[serviceID]
	if !knownService {
		return nil, false
	}
	applicationURL := "https://data.seoul.go.kr/together/mypage/actkeyReq_ss.do"
	family := "general"
	if credentialScope == "http://swopenapi.seoul.go.kr/api/subway/" {
		applicationURL = "https://data.seoul.go.kr/together/mypage/actkeyMetroReq_ss.do"
		family = "metro"
	}

	return &ExternalContract{
		Provider:          "Seoul Open Data Plaza",
		ProviderFamily:    family,
		ProviderServiceID: serviceID,
		DocumentationURL:  "https://data.seoul.go.kr/together/guide/useGuide.do",
		ApplicationURL:    applicationURL,
		AccessMode:        "provider_account_required",
		Auth: &ExternalAuthContract{
			Type:            "api_key",
			Placement:       "path",
			Name:            "KEY",
			CredentialScope: credentialScope,
		},
		InvocationState: InvocationBlockedInsecureTransport,
		VerifiedAt:      "2026-09-01",
	}, true
}
