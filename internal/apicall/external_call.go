package apicall

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"github.com/PuerkitoBio/goquery"
)

var (
	ErrExternalRedirect = errors.New("external provider redirect가 차단되었습니다")
	ErrExternalProvider = errors.New("external provider가 오류를 반환했습니다")
)

const (
	foodSchemaCacheTTL   = 5 * time.Minute
	foodSchemaCacheLimit = 128
)

// ExternalCredential is supplied by the local provider credential store. It is
// intentionally separate from ExternalContract so a spec can never serialize a
// secret. Domain is public VWorld key-registration metadata.
type ExternalCredential struct {
	Key    string
	Domain string
}

// ExternalCaller is the deep module behind LINK invocation. Callers provide a
// reviewed contract, operation name and ordinary parameters; provider endpoint
// construction, authentication placement, validation, redirect policy,
// redaction and response decoding remain inside this module.
type ExternalCaller struct {
	http            *http.Client
	inspectFood     foodSafetyInspector
	foodSchemaMu    sync.Mutex
	foodSchemaCache map[string]cachedFoodSchema
	now             func() time.Time
}

type cachedFoodSchema struct {
	schema    map[string]Param
	expiresAt time.Time
}

type foodSafetyInspector func(context.Context, *ExternalContract) (map[string]Param, error)

// NewExternalCaller builds the production caller. Redirects are rejected rather
// than followed so no header, query or path credential crosses an origin or
// path seam implicitly.
func NewExternalCaller() *ExternalCaller {
	client := fetch.NewHTTPClient(60 * time.Second)
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return newExternalCaller(client, nil)
}

func newExternalCaller(client *http.Client, inspector foodSafetyInspector) *ExternalCaller {
	if client == nil {
		return &ExternalCaller{}
	}
	isolated := *client
	isolated.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	caller := &ExternalCaller{
		http:            &isolated,
		inspectFood:     inspector,
		foodSchemaCache: make(map[string]cachedFoodSchema),
		now:             time.Now,
	}
	if caller.inspectFood == nil {
		caller.inspectFood = caller.inspectFoodSafetyKorea
	}
	return caller
}

// Call invokes one operation proved by the selected provider adapter.
func (c *ExternalCaller) Call(ctx context.Context, contract *ExternalContract, operation string, params map[string]string, credential ExternalCredential) (*CallResult, error) {
	if c == nil || c.http == nil {
		return nil, fmt.Errorf("external caller가 초기화되지 않았습니다")
	}
	if contract == nil || contract.Auth == nil {
		return nil, fmt.Errorf("검증된 external provider contract가 필요합니다")
	}
	if strings.TrimSpace(credential.Key) == "" {
		return nil, fmt.Errorf("%s provider credential이 비어 있습니다", contract.AdapterID)
	}
	var req *http.Request
	var err error
	switch contract.AdapterID {
	case "safetykorea":
		req, err = buildSafetyKoreaRequest(ctx, contract, operation, params, credential)
	case "foodsafetykorea":
		var schema map[string]Param
		schema, err = c.foodSafetySchema(ctx, contract)
		if err == nil {
			req, err = buildFoodSafetyKoreaRequest(ctx, contract, operation, params, credential, schema)
		}
	case "vworld":
		req, err = buildVWorldRequest(ctx, contract, operation, params, credential)
	default:
		return nil, fmt.Errorf("provider adapter %q의 자동 호출은 구현되지 않았습니다", contract.AdapterID)
	}
	if err != nil {
		return nil, err
	}
	result, err := c.do(req, credential.Key)
	if err != nil {
		return result, err
	}
	if providerErr := externalProviderError(contract, operation, result); providerErr != "" {
		return result, fmt.Errorf("%w: %s", ErrExternalProvider, providerErr)
	}
	return result, nil
}

func (c *ExternalCaller) foodSafetySchema(ctx context.Context, contract *ExternalContract) (map[string]Param, error) {
	cacheKey := fmt.Sprintf("%d\x00%s\x00%s", contract.AdapterRevision, contract.ProviderServiceID, contract.DocumentationURL)
	now := c.now()
	c.foodSchemaMu.Lock()
	if cached, ok := c.foodSchemaCache[cacheKey]; ok && now.Before(cached.expiresAt) {
		schema := cloneParamMap(cached.schema)
		c.foodSchemaMu.Unlock()
		return schema, nil
	}
	c.foodSchemaMu.Unlock()

	schema, err := c.inspectFood(ctx, contract)
	if err != nil {
		return nil, err
	}

	c.foodSchemaMu.Lock()
	for key, cached := range c.foodSchemaCache {
		if !now.Before(cached.expiresAt) {
			delete(c.foodSchemaCache, key)
		}
	}
	if len(c.foodSchemaCache) >= foodSchemaCacheLimit {
		var oldestKey string
		var oldestExpiry time.Time
		for key, cached := range c.foodSchemaCache {
			if oldestKey == "" || cached.expiresAt.Before(oldestExpiry) {
				oldestKey, oldestExpiry = key, cached.expiresAt
			}
		}
		delete(c.foodSchemaCache, oldestKey)
	}
	c.foodSchemaCache[cacheKey] = cachedFoodSchema{
		schema:    cloneParamMap(schema),
		expiresAt: now.Add(foodSchemaCacheTTL),
	}
	c.foodSchemaMu.Unlock()
	return schema, nil
}

func cloneParamMap(source map[string]Param) map[string]Param {
	clone := make(map[string]Param, len(source))
	for name, param := range source {
		clone[name] = param
	}
	return clone
}

func buildVWorldRequest(ctx context.Context, contract *ExternalContract, operation string, params map[string]string, credential ExternalCredential) (*http.Request, error) {
	if contract.Auth.Placement != "query" || contract.Auth.Name != "key" ||
		contract.Auth.CredentialScope != "https://api.vworld.kr/req/" {
		return nil, fmt.Errorf("VWorld credential contract가 현재 구현과 일치하지 않습니다")
	}
	if err := validateExternalParam("VWorld key", credential.Key); err != nil {
		return nil, err
	}
	if credential.Domain != "" {
		domain, err := url.Parse(credential.Domain)
		if err != nil || (domain.Scheme != "https" && domain.Scheme != "http") || domain.Hostname() == "" || domain.User != nil || domain.Fragment != "" {
			return nil, fmt.Errorf("VWorld 등록 domain이 유효한 HTTP(S) URL이 아닙니다")
		}
	}
	spec, ok := vWorldOperationFor(contract.ProviderFamily, operation)
	if !ok {
		if len(vWorldOperationRegistry[contract.ProviderFamily]) == 0 {
			return nil, fmt.Errorf("VWorld family %q은 자동 호출 대상이 아닙니다", contract.ProviderFamily)
		}
		return nil, fmt.Errorf("VWorld %s operation %q은 지원하지 않습니다", contract.ProviderFamily, operation)
	}
	allowed := make(map[string]bool, len(spec.operation.Params))
	required := make([]string, 0, len(spec.operation.Params))
	for _, param := range spec.operation.Params {
		allowed[param.Name] = true
		if isRequired(param.Required) {
			required = append(required, param.Name)
		}
	}
	if contract.ProviderFamily == "data" && operation == "GetFeature" &&
		strings.TrimSpace(params["geomFilter"]) == "" && strings.TrimSpace(params["attrFilter"]) == "" {
		return nil, fmt.Errorf("VWorld GetFeature는 geomFilter 또는 attrFilter 중 하나가 필요합니다")
	}
	dataCode := ""
	if contract.ProviderFamily == "data" {
		var ok bool
		dataCode, ok = vWorldDataCodes[contract.ProviderServiceID]
		if !ok {
			return nil, fmt.Errorf("VWorld data service %q의 고정 data 식별자가 검증되지 않았습니다", contract.ProviderServiceID)
		}
	}
	if contract.ProviderFamily == "search" {
		typeValue := strings.ToLower(params["type"])
		if (typeValue == "address" || typeValue == "district") && strings.TrimSpace(params["category"]) == "" {
			return nil, fmt.Errorf("VWorld search type=%s에는 category가 필요합니다", typeValue)
		}
	}
	if contract.ProviderFamily != "data" && contract.ProviderFamily != "address" && contract.ProviderFamily != "search" && contract.ProviderFamily != "ogc" {
		return nil, fmt.Errorf("VWorld family %q은 자동 호출 대상이 아닙니다", contract.ProviderFamily)
	}
	for name, value := range params {
		if name == "key" || name == "domain" || name == "service" || name == "request" || name == "data" {
			return nil, fmt.Errorf("%s는 odeduck이 주입하므로 params에 넣지 마세요", name)
		}
		if !allowed[name] {
			return nil, fmt.Errorf("%s는 VWorld %s의 공식 요청변수가 아닙니다", name, operation)
		}
		if err := validateExternalParam(name, value); err != nil {
			return nil, err
		}
		if options := spec.enums[name]; len(options) > 0 && !options[value] && !options[strings.ToLower(value)] {
			return nil, fmt.Errorf("%s=%q은 공식 enum에 없습니다", name, value)
		}
	}
	for _, name := range required {
		if strings.TrimSpace(params[name]) == "" {
			return nil, fmt.Errorf("VWorld %s에는 %s가 필요합니다", operation, name)
		}
	}
	if err := validateBoundedPositive(params, "size", 1000); err != nil {
		return nil, err
	}
	if err := validateBoundedPositive(params, "page", 0); err != nil {
		return nil, err
	}
	if contract.ProviderFamily == "ogc" {
		if err := validateBoundedPositive(params, "width", 2048); err != nil {
			return nil, err
		}
		if err := validateBoundedPositive(params, "height", 2048); err != nil {
			return nil, err
		}
		if err := validateBoundedPositive(params, "feature_count", 1000); err != nil {
			return nil, err
		}
		if err := validateBoundedPositive(params, "count", 1000); err != nil {
			return nil, err
		}
		if err := validateBoundedPositive(params, "maxfeatures", 1000); err != nil {
			return nil, err
		}
		for _, name := range []string{"i", "j", "startindex"} {
			if err := validateBoundedNonNegative(params, name); err != nil {
				return nil, err
			}
		}
	}
	u := &url.URL{Scheme: "https", Host: "api.vworld.kr", Path: spec.path}
	q := u.Query()
	q.Set("service", spec.service)
	q.Set("request", spec.request)
	q.Set("key", credential.Key)
	if dataCode != "" {
		q.Set("data", dataCode)
	}
	if credential.Domain != "" {
		q.Set("domain", credential.Domain)
	}
	for name, value := range params {
		q.Set(name, value)
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, redactURLConstructionError(err, credential.Key)
	}
	req.Header.Set("Accept", "application/json,application/xml,image/png")
	req.Header.Set("User-Agent", fetch.DefaultUserAgent)
	return req, nil
}

func validateBoundedNonNegative(params map[string]string, name string) error {
	value, ok := params[name]
	if !ok {
		return nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return fmt.Errorf("%s는 0 이상의 정수여야 합니다", name)
	}
	return nil
}

func validateBoundedPositive(params map[string]string, name string, max int) error {
	value, ok := params[name]
	if !ok {
		return nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 || (max > 0 && n > max) {
		if max > 0 {
			return fmt.Errorf("%s는 1~%d 정수여야 합니다", name, max)
		}
		return fmt.Errorf("%s는 1 이상의 정수여야 합니다", name)
	}
	return nil
}

var foodSafetyCredential = regexp.MustCompile(`^[A-Za-z0-9_-]{8,256}$`)
var foodSafetyParameterName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,127}$`)

func buildFoodSafetyKoreaRequest(ctx context.Context, contract *ExternalContract, operation string, params map[string]string, credential ExternalCredential, schema map[string]Param) (*http.Request, error) {
	if contract.Auth.Placement != "path" || contract.Auth.Name != "keyId" ||
		contract.Auth.CredentialScope != "https://openapi.foodsafetykorea.go.kr/api/" ||
		!foodSafetyKoreaServiceID.MatchString(contract.ProviderServiceID) {
		return nil, fmt.Errorf("FoodSafetyKorea credential/service contract가 현재 구현과 일치하지 않습니다")
	}
	if operation != foodSafetyKoreaListOperation {
		return nil, fmt.Errorf("FoodSafetyKorea operation %q은 지원하지 않습니다 (list만 지원)", operation)
	}
	if !foodSafetyCredential.MatchString(credential.Key) {
		return nil, fmt.Errorf("FoodSafetyKorea key는 8~256자의 영문·숫자·_- 형식이어야 합니다")
	}
	dataType := strings.ToLower(params["dataType"])
	if dataType != "json" && dataType != "xml" {
		return nil, fmt.Errorf("dataType은 json 또는 xml이어야 합니다")
	}
	start, startErr := strconv.Atoi(params["startIdx"])
	end, endErr := strconv.Atoi(params["endIdx"])
	if startErr != nil || endErr != nil || start < 1 || end < start || end-start+1 > 1000 {
		return nil, fmt.Errorf("startIdx/endIdx는 1 이상이며 한 요청 범위가 1,000건 이하여야 합니다")
	}
	for _, param := range foodSafetyKoreaBaseParams {
		if _, ok := schema[param.Name]; !ok {
			return nil, fmt.Errorf("FoodSafetyKorea 공식 요청인자 표에서 %s를 확인하지 못했습니다", param.Name)
		}
	}
	filterNames := make([]string, 0, len(params)-3)
	for name, value := range params {
		if name == "dataType" || name == "startIdx" || name == "endIdx" {
			continue
		}
		if name == "keyId" || name == "serviceId" {
			return nil, fmt.Errorf("%s는 odeduck이 주입하므로 params에 넣지 마세요", name)
		}
		if _, ok := schema[name]; !ok {
			return nil, fmt.Errorf("%s는 공식 요청인자 표에 없는 filter입니다", name)
		}
		if err := validateExternalParam(name, value); err != nil {
			return nil, err
		}
		filterNames = append(filterNames, name)
	}
	sort.Strings(filterNames)
	rawURL := "https://openapi.foodsafetykorea.go.kr/api/" + credential.Key + "/" +
		contract.ProviderServiceID + "/" + dataType + "/" + strconv.Itoa(start) + "/" + strconv.Itoa(end)
	if len(filterNames) > 0 {
		filters := make([]string, 0, len(filterNames))
		for _, name := range filterNames {
			filters = append(filters, name+"="+strings.ReplaceAll(url.QueryEscape(params[name]), "+", "%20"))
		}
		rawURL += "/" + strings.Join(filters, "&")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, redactURLConstructionError(err, credential.Key)
	}
	req.Header.Set("Accept", "application/json,application/xml")
	req.Header.Set("User-Agent", fetch.DefaultUserAgent)
	return req, nil
}

func redactURLConstructionError(err error, secret string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("external request URL 생성 실패: %s", redactSecret(err.Error(), secret))
}

func (c *ExternalCaller) inspectFoodSafetyKorea(ctx context.Context, contract *ExternalContract) (map[string]Param, error) {
	u, err := url.Parse(contract.DocumentationURL)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || !strings.EqualFold(u.Host, "www.foodsafetykorea.go.kr") ||
		u.Path != "/api/openApiInfo.do" || u.Query().Get("svc_no") != contract.ProviderServiceID || u.User != nil || u.Fragment != "" {
		return nil, fmt.Errorf("FoodSafetyKorea documentation URL이 검증된 service detail 형식이 아닙니다")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/html")
	req.Header.Set("User-Agent", fetch.DefaultUserAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("FoodSafetyKorea 요청인자 검사 실패: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("FoodSafetyKorea 요청인자 검사 HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, fetch.DefaultMaxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > fetch.DefaultMaxResponseBytes {
		return nil, fetch.ErrResponseTooLarge
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	if serviceID, ok := doc.Find("#svc_no").First().Attr("value"); !ok || strings.TrimSpace(serviceID) != contract.ProviderServiceID {
		return nil, fmt.Errorf("FoodSafetyKorea detail page의 service ID가 일치하지 않습니다")
	}
	var schema map[string]Param
	doc.Find("table").EachWithBreak(func(_ int, table *goquery.Selection) bool {
		if cleanText(table.Find("caption").First().Text()) != "요청인자" {
			return true
		}
		candidate := map[string]Param{}
		table.Find("tbody tr").Each(func(_ int, row *goquery.Selection) {
			cells := row.Find("td")
			if cells.Length() < 4 {
				return
			}
			name := cleanText(cells.Eq(1).Text())
			if !foodSafetyParameterName.MatchString(name) {
				return
			}
			typeText := cleanText(cells.Eq(2).Text())
			candidate[name] = Param{Name: name, Required: map[bool]string{true: "필수", false: "선택"}[strings.Contains(typeText, "필수")], Desc: cleanText(cells.Eq(3).Text())}
		})
		if candidate["keyId"].Name != "" && candidate["serviceId"].Name != "" &&
			candidate["dataType"].Name != "" && candidate["startIdx"].Name != "" && candidate["endIdx"].Name != "" {
			schema = candidate
			return false
		}
		return true
	})
	if len(schema) == 0 {
		return nil, fmt.Errorf("FoodSafetyKorea 공식 페이지에서 요청인자 표를 찾지 못했습니다")
	}
	return schema, nil
}

func buildSafetyKoreaRequest(ctx context.Context, contract *ExternalContract, operation string, params map[string]string, credential ExternalCredential) (*http.Request, error) {
	if contract.Auth.Placement != "header" || contract.Auth.Name != "AuthKey" ||
		contract.Auth.CredentialScope != "https://www.safetykorea.kr/openapi/api/" {
		return nil, fmt.Errorf("SafetyKorea credential contract가 현재 구현과 일치하지 않습니다")
	}
	spec, ok := safetyKoreaOperationFor(operation)
	if !ok {
		return nil, fmt.Errorf("SafetyKorea operation %q은 지원하지 않습니다", operation)
	}
	required := make([]string, 0, len(spec.operation.Params))
	for _, param := range spec.operation.Params {
		if isRequired(param.Required) {
			required = append(required, param.Name)
		}
	}
	if len(params) != len(required) {
		return nil, fmt.Errorf("%s 요청변수는 %s만 허용합니다", operation, strings.Join(required, ", "))
	}
	for _, name := range required {
		if err := validateExternalParam(name, params[name]); err != nil {
			return nil, err
		}
	}
	if len(spec.keyOptions) > 0 && !spec.keyOptions[params["conditionKey"]] {
		return nil, fmt.Errorf("%s conditionKey %q은 공식 enum에 없습니다", operation, params["conditionKey"])
	}
	u := &url.URL{Scheme: "https", Host: "www.safetykorea.kr", Path: spec.path}
	q := u.Query()
	for _, name := range required {
		q.Set(name, params[name])
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("AuthKey", credential.Key)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", fetch.DefaultUserAgent)
	return req, nil
}

func stringSet(values ...string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func validateExternalParam(name, value string) error {
	if strings.TrimSpace(value) == "" || len(value) > 4096 || strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("%s는 비어 있지 않은 4096자 이하 단일 값이어야 합니다", name)
	}
	return nil
}

func (c *ExternalCaller) do(req *http.Request, secret string) (*CallResult, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("external call %s: %s", requestTargetWithoutSecret(req, secret), redactSecret(err.Error(), secret))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, fetch.DefaultMaxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > fetch.DefaultMaxResponseBytes {
		return nil, fmt.Errorf("%w: external provider (%d bytes 제한)", fetch.ErrResponseTooLarge, fetch.DefaultMaxResponseBytes)
	}
	result := &CallResult{
		Status:      resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		rawBody:     append([]byte(nil), body...),
	}
	if strings.HasPrefix(strings.ToLower(result.ContentType), "image/") || strings.HasPrefix(strings.ToLower(result.ContentType), "application/octet-stream") {
		result.BodyEncoding = "base64"
		result.Body = base64.StdEncoding.EncodeToString(body)
	} else {
		result.Body = decodeBody(result.ContentType, body)
	}
	// Preserve source bytes, or withhold the entire response. Redaction would
	// invent replacement observations and miss JSON/XML-escaped credentials.
	if echoesCredential(result.Body, secret) || echoesCredential(string(body), secret) || echoesCredential(result.ContentType, secret) {
		return nil, errCredentialResponse
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return result, fmt.Errorf("%w: HTTP %d", ErrExternalRedirect, resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, fmt.Errorf("%w: external provider status %d", ErrHTTPStatus, resp.StatusCode)
	}
	return result, nil
}

func externalProviderError(contract *ExternalContract, operation string, result *CallResult) string {
	if result == nil {
		return "provider 응답이 없습니다"
	}
	root, structured := result.Body.(map[string]any)
	switch contract.AdapterID {
	case "safetykorea":
		if !structured {
			return "SafetyKorea 성공 envelope를 확인할 수 없습니다"
		}
		code, exists := root["resultCode"]
		if !exists {
			return "SafetyKorea resultCode가 없습니다"
		}
		if fmt.Sprint(code) == "2000" {
			return ""
		}
		return fmt.Sprintf("resultCode=%v resultMsg=%v", code, root["resultMsg"])
	case "foodsafetykorea":
		if !structured {
			return "FoodSafetyKorea 성공 envelope를 확인할 수 없습니다"
		}
		service := root
		if wrapped, ok := root[contract.ProviderServiceID].(map[string]any); ok {
			service = wrapped
		}
		result, _ := service["RESULT"].(map[string]any)
		code, exists := result["CODE"]
		if !exists {
			return "FoodSafetyKorea RESULT.CODE가 없습니다"
		}
		if fmt.Sprint(code) == "INFO-000" {
			return ""
		}
		return fmt.Sprintf("CODE=%v MSG=%v", code, result["MSG"])
	case "vworld":
		if contract.ProviderFamily == "ogc" {
			return vWorldOGCError(operation, result, root, structured)
		}
		if !structured {
			return "VWorld 성공 envelope를 확인할 수 없습니다"
		}
		response := root
		if wrapped, ok := root["response"].(map[string]any); ok {
			response = wrapped
		}
		status, exists := response["status"]
		if !exists {
			return "VWorld response.status가 없습니다"
		}
		if strings.EqualFold(fmt.Sprint(status), "OK") {
			return ""
		}
		return fmt.Sprintf("status=%v error=%v", status, response["error"])
	}
	return "알 수 없는 provider adapter 응답입니다"
}

func vWorldOGCError(operation string, result *CallResult, root map[string]any, structured bool) string {
	if structured {
		response := root
		if wrapped, ok := root["response"].(map[string]any); ok {
			response = wrapped
		}
		if status, ok := response["status"]; ok {
			if strings.EqualFold(fmt.Sprint(status), "OK") {
				return ""
			}
			return fmt.Sprintf("status=%v error=%v", status, response["error"])
		}
	}
	if operation == "wms.GetMap" && result.BodyEncoding == "base64" && strings.HasPrefix(strings.ToLower(result.ContentType), "image/") {
		return ""
	}
	rootName := externalXMLRoot(result.rawBody)
	if strings.Contains(strings.ToLower(rootName), "exception") {
		return fmt.Sprintf("VWorld OGC %s", rootName)
	}
	wantRoots := map[string]map[string]bool{
		"wms.GetCapabilities": {"WMS_Capabilities": true, "WMT_MS_Capabilities": true},
		"wms.GetFeatureInfo":  {"FeatureInfoResponse": true, "FeatureCollection": true},
		"wfs.GetFeature":      {"FeatureCollection": true},
		"wfs.GetCapabilities": {"WFS_Capabilities": true},
	}
	if wantRoots[operation][rootName] {
		return ""
	}
	return fmt.Sprintf("VWorld OGC %s 성공 envelope를 확인할 수 없습니다", operation)
}

func externalXMLRoot(body []byte) string {
	decoder := xml.NewDecoder(strings.NewReader(string(body)))
	for {
		token, err := decoder.Token()
		if err != nil {
			return ""
		}
		if start, ok := token.(xml.StartElement); ok {
			return start.Name.Local
		}
	}
}

func requestTargetWithoutSecret(req *http.Request, secret string) string {
	if req == nil || req.URL == nil {
		return "provider"
	}
	return redactSecret(req.URL.String(), secret)
}

func redactSecret(value, secret string) string {
	if secret == "" {
		return value
	}
	variants := []string{
		secret,
		url.QueryEscape(secret),
		strings.ReplaceAll(url.QueryEscape(secret), "+", "%20"),
		url.PathEscape(secret),
	}
	for _, variant := range variants {
		if variant != "" {
			value = strings.ReplaceAll(value, variant, "REDACTED")
			value = strings.ReplaceAll(value, strings.ToLower(variant), "REDACTED")
		}
	}
	return value
}
