package apicall

import "net/url"

// safetyKoreaAdapter is the strictest reference shape: one HTTPS hub path, no
// query parameters, and a provider-scoped header credential.
type safetyKoreaAdapter struct{}

type safetyKoreaOperationSpec struct {
	operation  ExternalOperation
	path       string
	keyOptions map[string]bool
}

var safetyKoreaOperationRegistry = []safetyKoreaOperationSpec{
	{
		operation: ExternalOperation{Name: "certificationList", Description: "KC 인증정보 검색", ResponseKind: "json", Params: []Param{
			{Name: "conditionKey", Required: "필수", Desc: "all|certNum|productName|modelName|certDate|signDate"},
			{Name: "conditionValue", Required: "필수", Desc: "검색어"},
		}},
		path:       "/openapi/api/cert/certificationList.json",
		keyOptions: stringSet("all", "certNum", "productName", "modelName", "certDate", "signDate"),
	},
	{
		operation: ExternalOperation{Name: "certificationDetail", Description: "KC 인증정보 상세", ResponseKind: "json", Params: []Param{
			{Name: "certNum", Required: "필수", Desc: "인증번호"},
		}},
		path: "/openapi/api/cert/certificationDetail.json",
	},
	{
		operation: ExternalOperation{Name: "recallList", Description: "국내 리콜정보 검색", ResponseKind: "json", Params: []Param{
			{Name: "conditionKey", Required: "필수", Desc: "all|barcodeNum|recallProductName|recallBrandName|recallModelName|certNum|publishDate"},
			{Name: "conditionValue", Required: "필수", Desc: "검색어"},
		}},
		path:       "/openapi/api/recall/recallList.json",
		keyOptions: stringSet("all", "barcodeNum", "recallProductName", "recallBrandName", "recallModelName", "certNum", "publishDate"),
	},
	{
		operation: ExternalOperation{Name: "recallDetail", Description: "국내 리콜정보 상세", ResponseKind: "json", Params: []Param{
			{Name: "recallUid", Required: "필수", Desc: "리콜 아이디"},
		}},
		path: "/openapi/api/recall/recallDetail.json",
	},
	{
		operation: ExternalOperation{Name: "fRecallList", Description: "국외 리콜정보 검색", ResponseKind: "json", Params: []Param{
			{Name: "conditionKey", Required: "필수", Desc: "all|recallProductName|recallBrandName|recallModelName|publishDate|fRecallUid"},
			{Name: "conditionValue", Required: "필수", Desc: "검색어"},
		}},
		path:       "/openapi/api/recall/fRecallList.json",
		keyOptions: stringSet("all", "recallProductName", "recallBrandName", "recallModelName", "publishDate", "fRecallUid"),
	},
}

func safetyKoreaOperationFor(name string) (safetyKoreaOperationSpec, bool) {
	for _, spec := range safetyKoreaOperationRegistry {
		if spec.operation.Name == name {
			return spec, true
		}
	}
	return safetyKoreaOperationSpec{}, false
}

func safetyKoreaOperations() []ExternalOperation {
	operations := make([]ExternalOperation, 0, len(safetyKoreaOperationRegistry))
	for _, spec := range safetyKoreaOperationRegistry {
		operation := spec.operation
		operation.Params = append([]Param(nil), operation.Params...)
		operations = append(operations, operation)
	}
	return operations
}

func (safetyKoreaAdapter) ContractFor(target *url.URL) (*ExternalContract, bool) {
	host, path, ok := providerURLShape(target)
	if !ok || !isHTTPS(target) || target.RawQuery != "" ||
		host != "www.safetykorea.kr" ||
		path != "/release/openapi" {
		return nil, false
	}
	return &ExternalContract{
		Provider:             "SafetyKorea",
		ProviderFamily:       "product-safety",
		DocumentationURL:     "https://www.safetykorea.kr/resources/fileData/openapi/Open_API_%EC%82%AC%EC%9A%A9%EC%84%A4%EB%AA%85%EC%84%9C_v2.0.pdf",
		DocumentationVersion: "2.0 (2025-06-30)",
		ApplicationURL:       "https://www.safetykorea.kr/release/openapi2",
		AccessMode:           "manual_approval",
		Auth: &ExternalAuthContract{
			Type:            "api_key",
			Placement:       "header",
			Name:            "AuthKey",
			CredentialScope: "https://www.safetykorea.kr/openapi/api/",
		},
		InvocationState: InvocationImplemented,
		Operations:      safetyKoreaOperations(),
		VerifiedAt:      "2026-09-01",
	}, true
}
