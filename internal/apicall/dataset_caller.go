package apicall

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"github.com/JungHoonGhae/odeduck/internal/portal"
)

// CredentialSource keeps secret acquisition outside the invocation module. The
// production adapter reads data.go.kr's refreshed account key or a locally
// stored provider-scoped key; tests use an in-memory adapter.
type CredentialSource interface {
	DataGoKR(context.Context) (string, error)
	InvalidateDataGoKR()
	External(context.Context, string, string) (key, domain string, err error)
}

type DatasetCallRequest struct {
	PK        string
	Operation string
	Params    map[string]string
	Wait      time.Duration
	OnWait    func(elapsed, remaining time.Duration)
}

// DatasetCaller is the single invocation interface used by CLI and MCP. It
// dispatches a data.go.kr REST dataset or an implemented LINK provider without
// asking either caller to understand credential placement or endpoint shape.
type DatasetCaller struct {
	fetch       *fetch.Client
	baseURL     string
	credentials CredentialSource
	external    *ExternalCaller
	rest        restDatasetCall
}

type restDatasetCall func(context.Context, *fetch.Client, string, map[string]string, string, time.Duration, func(time.Duration, time.Duration)) (*CallResult, error)

func NewDatasetCaller(f *fetch.Client, baseURL string, credentials CredentialSource) *DatasetCaller {
	return newDatasetCaller(f, baseURL, credentials, NewExternalCaller())
}

func newDatasetCaller(f *fetch.Client, baseURL string, credentials CredentialSource, external *ExternalCaller) *DatasetCaller {
	if baseURL == "" {
		baseURL = portal.BaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &DatasetCaller{fetch: f, baseURL: baseURL, credentials: credentials, external: external, rest: callRESTDataset}
}

func callRESTDataset(ctx context.Context, f *fetch.Client, endpoint string, params map[string]string, key string, wait time.Duration, onWait func(time.Duration, time.Duration)) (*CallResult, error) {
	if wait <= 0 {
		return Call(ctx, f, endpoint, params, key)
	}
	return CallWaiting(ctx, f, endpoint, params, key, wait, onWait)
}

func (c *DatasetCaller) Call(ctx context.Context, request DatasetCallRequest) (*CallResult, error) {
	if c == nil || c.fetch == nil || c.credentials == nil || c.rest == nil {
		return nil, fmt.Errorf("dataset caller가 초기화되지 않았습니다")
	}
	if request.PK == "" {
		return nil, fmt.Errorf("publicDataPk가 필요합니다")
	}
	spec, err := DescribeCatalogued(ctx, c.fetch, c.baseURL, request.PK)
	if err != nil {
		return nil, err
	}
	if isLinkAPIType(spec.APIType) {
		return c.callExternal(ctx, spec, request)
	}
	operation, err := resolveOperation(spec, request.PK, request.Operation)
	if err != nil {
		return nil, err
	}
	if !hasDetailedInvocationEvidence(spec, operation) {
		return nil, fmt.Errorf("pk=%s의 필수 요청변수를 확인할 상세 호출 계약이 없어 credential 호출을 차단했습니다 — inspect_dataset을 다시 실행하거나 공식 명세를 확인하세요", request.PK)
	}
	if missing := MissingRequired(operation, request.Params); len(missing) > 0 {
		return nil, fmt.Errorf("필수 요청변수가 빠졌습니다: %s — inspect_dataset(pk=%s)으로 확인하세요", strings.Join(missing, ", "), request.PK)
	}
	key, err := c.credentials.DataGoKR(ctx)
	if err != nil {
		return nil, fmt.Errorf("data.go.kr 인증키를 얻지 못했습니다: %w", err)
	}
	doCall := func(candidate string) (*CallResult, error) {
		return c.rest(ctx, c.fetch, operation.Endpoint, request.Params, candidate, request.Wait, request.OnWait)
	}
	result, callErr := doCall(key)
	if errors.Is(callErr, ErrKeyRejected) {
		c.credentials.InvalidateDataGoKR()
		if fresh, refreshErr := c.credentials.DataGoKR(ctx); refreshErr == nil && fresh != key {
			result, callErr = doCall(fresh)
		}
	}
	if result != nil {
		result.Delivery = "REST"
		result.Operation = operation.Name
	}
	return result, callErr
}

func hasDetailedInvocationEvidence(spec *APISpec, operation *Operation) bool {
	if spec == nil || spec.OfficialAPI == nil {
		// Legacy/web-only descriptions are themselves the detailed contract.
		return true
	}
	if operation == nil {
		return false
	}
	if operation.ContractKind == ContractKindOfficialSwagger {
		// Operation-scoped Swagger evidence can legitimately describe a
		// parameterless operation. A different bulk-only operation does not
		// inherit this authority merely because it is in the same APISpec.
		return true
	}
	if operation.ContractKind != ContractKindFirstPartyWebContract || spec.EndpointOnly || strings.TrimSpace(operation.Endpoint) == "" || len(operation.Params) == 0 {
		return false
	}
	// Merely reaching an HTML page is not a detailed invocation contract. Every
	// parameter merged into the selected operation must have a known
	// required/optional classification; bulk-only rows use ParamRequirementUnknown and remain
	// fail-closed until the page proves those facts.
	for _, param := range operation.Params {
		required := strings.TrimSpace(param.Required)
		if strings.TrimSpace(param.Name) == "" || required == ParamRequirementUnknown {
			return false
		}
	}
	return true
}

func (c *DatasetCaller) callExternal(ctx context.Context, spec *APISpec, request DatasetCallRequest) (*CallResult, error) {
	if spec.Handoff == nil || spec.Handoff.Contract == nil || spec.Handoff.State != HandoffContractKnown {
		return nil, fmt.Errorf("pk=%s LINK provider 계약이 검사되지 않아 자동 호출하지 않습니다", request.PK)
	}
	contract := spec.Handoff.Contract
	switch contract.InvocationState {
	case InvocationImplemented:
	case InvocationBlockedInsecureTransport:
		return nil, fmt.Errorf("pk=%s %s 호출은 credential을 보호할 HTTPS endpoint가 없어 차단되었습니다", request.PK, contract.Provider)
	default:
		return nil, fmt.Errorf("pk=%s %s/%s 자동 호출은 아직 구현되지 않았습니다", request.PK, contract.Provider, contract.ProviderFamily)
	}
	if c.external == nil {
		return nil, fmt.Errorf("external caller가 초기화되지 않았습니다")
	}
	operation, err := resolveExternalOperation(contract, request.Operation)
	if err != nil {
		return nil, err
	}
	if contract.Auth == nil {
		return nil, fmt.Errorf("%s credential contract가 없습니다", contract.Provider)
	}
	key, domain, err := c.credentials.External(ctx, contract.AdapterID, contract.Auth.CredentialScope)
	if err != nil {
		return nil, fmt.Errorf("%s credential을 얻지 못했습니다: %w (신청: %s)", contract.Provider, err, contract.ApplicationURL)
	}
	result, callErr := c.external.Call(ctx, contract, operation, request.Params, ExternalCredential{Key: key, Domain: domain})
	if result != nil {
		result.Delivery = "LINK"
		result.Operation = operation
	}
	return result, callErr
}

func resolveExternalOperation(contract *ExternalContract, requested string) (string, error) {
	if contract == nil || len(contract.Operations) == 0 {
		return "", fmt.Errorf("external provider contract에 호출 operation이 없습니다")
	}
	if requested == "" {
		if len(contract.Operations) == 1 {
			return contract.Operations[0].Name, nil
		}
		names := make([]string, 0, len(contract.Operations))
		for _, operation := range contract.Operations {
			names = append(names, operation.Name)
		}
		return "", fmt.Errorf("상세기능이 여러 개입니다 — --op 중 하나를 지정하세요: %s", strings.Join(names, ", "))
	}
	for _, operation := range contract.Operations {
		if operation.Name == requested {
			return requested, nil
		}
	}
	return "", fmt.Errorf("operation %q은 provider contract에 없습니다 — inspect_dataset으로 다시 확인하세요", requested)
}
