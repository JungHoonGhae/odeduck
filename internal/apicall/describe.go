// Package apicall surfaces data.go.kr OpenAPI specs to an agent (describe) and
// performs authenticated calls (call). It starts with documented bulk API
// metadata retained in the local catalogue and uses page contracts only for
// facts that interface omits. Uncertain structure is surfaced rather than
// guessed.
package apicall

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/JungHoonGhae/oddsock/internal/catalog"
	"github.com/JungHoonGhae/oddsock/internal/fetch"
	"github.com/JungHoonGhae/oddsock/internal/portal"
	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/idna"
)

// APISpec is the surfaced view of one dataset's OpenAPI detail page.
type APISpec struct {
	PublicDataPk string      `json:"publicDataPk"`
	DataName     string      `json:"dataName"`
	Operations   []Operation `json:"operations"`
	GuideDoc     string      `json:"guideDoc,omitempty"`    // 참고문서 file name, NOT parsed
	GuideDocURL  string      `json:"guideDocUrl,omitempty"` // where to fetch that file
	// APIType is the portal's own 'API 유형' field: REST means the page documents
	// operations and request variables; LINK means the portal only points at the
	// publisher's own site, so no spec exists here to read. Knowing which is
	// which decides whether describe→call can be driven from this page at all.
	APIType string `json:"apiType,omitempty"`
	// EndpointOnly marks a spec whose endpoint was recovered from the page but
	// whose request parameters the portal never documents.
	EndpointOnly bool `json:"endpointOnly,omitempty"`
	// LinkURL is the publisher's own page for a LINK dataset, taken from the
	// legacy URL row or the KRDS link lookup. The portal has it on every LINK page
	// sampled, so telling a
	// caller to "check the publisher's documentation" without handing over the
	// address it already holds is withholding the one actionable thing on the page.
	// oddsock does not follow it: the publishers are a long tail (39 distinct hosts
	// in 70 sampled datasets, the largest 13%), each with its own registration and
	// spec format, so reading it is the agent's job — surfacing it is ours.
	LinkURL string           `json:"linkUrl,omitempty"`
	Handoff *ExternalHandoff `json:"handoff,omitempty"`
	// Approval is the portal's 심의유형 row: whether an application is granted
	// automatically or waits for a human at the publishing agency.
	Approval *Approval `json:"approval,omitempty"`
	// Note is set only when the spec is incomplete, to say where the rest of it
	// lives. Without it an empty Operations list is a dead end.
	Note string `json:"note,omitempty"`
	// OfficialAPI preserves the documented bulk-catalogue facts used before the
	// portal page fallback. Evidence makes the boundary visible to callers.
	OfficialAPI *catalog.OfficialAPIContract `json:"officialApi,omitempty"`
	Evidence    []SpecEvidence               `json:"evidence,omitempty"`
	Warnings    []string                     `json:"warnings,omitempty"`
}

type SpecEvidence struct {
	Purpose   string `json:"purpose"`
	Kind      string `json:"kind"`
	URL       string `json:"url"`
	Stability string `json:"stability"`
}

// ContractKind identifies the evidence that authorizes one operation. Keeping
// it operation-scoped prevents a documented operation from lending credential
// authority to an unrelated bulk-only row in the same dataset.
type ContractKind string

const (
	ContractKindOfficialAPI           ContractKind = "official_api"
	ContractKindOfficialSwagger       ContractKind = "official_swagger"
	ContractKindFirstPartyWebContract ContractKind = "first_party_web_contract"
	ParamRequirementUnknown           string       = "미제공"
)

// ExternalHandoff is the only contract shared by heterogeneous LINK datasets.
// URL is an official starting point resolved through data.go.kr, not necessarily
// an API endpoint or even a specification page. State and NextAction keep an
// agent from sending call_api parameters to an uninspected provider page. A
// failed portal lookup uses resolution_failed plus a structured Failure instead
// of looking like a successful LINK response with an unexplained empty URL.
type ExternalHandoff struct {
	URL         string            `json:"url,omitempty"`
	Host        string            `json:"host,omitempty"`
	Trust       string            `json:"trust,omitempty"`
	FetchPolicy string            `json:"fetchPolicy,omitempty"`
	State       string            `json:"state"`      // inspection_required | contract_known | resolution_failed
	NextAction  string            `json:"nextAction"` // inspect_provider_contract | request_provider_access | retry_link_resolution | choose_another_dataset
	Contract    *ExternalContract `json:"contract,omitempty"`
	Failure     *HandoffFailure   `json:"failure,omitempty"`
}

// HandoffFailure keeps LINK resolution failures machine-actionable without
// pretending a missing URL is the same thing as an unsupported provider.
type HandoffFailure struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

const (
	HandoffInspectionRequired  = "inspection_required"
	HandoffInspectContract     = "inspect_provider_contract"
	HandoffContractKnown       = "contract_known"
	HandoffRequestAccess       = "request_provider_access"
	HandoffUseProviderDirectly = "use_provider_directly"
	HandoffPublisherUntrusted  = "publisher_supplied_untrusted"
	HandoffSafeFetcherRequired = "safe_fetcher_required"
	HandoffResolutionFailed    = "resolution_failed"
	HandoffRetryResolution     = "retry_link_resolution"
	HandoffChooseAnother       = "choose_another_dataset"
)

type linkResolutionError struct {
	code      string
	retryable bool
	err       error
}

func (e *linkResolutionError) Error() string { return e.err.Error() }
func (e *linkResolutionError) Unwrap() error { return e.err }

// ExternalContract records facts from a versioned provider document. AdapterID
// and AdapterRevision identify our interpretation; DocumentationVersion and
// VerifiedAt track the publisher evidence independently. None of these implies
// that oddsock can invoke the provider: InvocationState says whether a
// provider-specific credential and caller have actually been wired.
type ExternalContract struct {
	AdapterID            string                `json:"adapterId"`
	AdapterRevision      int                   `json:"adapterRevision"`
	Provider             string                `json:"provider"`
	ProviderFamily       string                `json:"providerFamily,omitempty"`
	ProviderServiceID    string                `json:"providerServiceId,omitempty"`
	DocumentationURL     string                `json:"documentationUrl"`
	DocumentationVersion string                `json:"documentationVersion,omitempty"`
	ApplicationURL       string                `json:"applicationUrl,omitempty"`
	AccessMode           string                `json:"accessMode"`
	Auth                 *ExternalAuthContract `json:"auth,omitempty"`
	InvocationState      string                `json:"invocationState"`
	Operations           []ExternalOperation   `json:"operations,omitempty"`
	VerifiedAt           string                `json:"verifiedAt"`
}

const (
	InvocationImplemented              = "implemented"
	InvocationNotImplemented           = "not_implemented"
	InvocationBlockedInsecureTransport = "blocked_insecure_transport"
)

// ExternalOperation is the typed invocation surface proven by the provider's
// own documentation. Endpoint construction and credentials stay private.
type ExternalOperation struct {
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	Params        []Param `json:"params,omitempty"`
	ResponseKind  string  `json:"responseKind"`
	DynamicParams bool    `json:"dynamicParams,omitempty"`
}

// ExternalAuthContract describes where the external provider expects its own
// credential. It is metadata only; no credential value enters APISpec.
type ExternalAuthContract struct {
	Type            string `json:"type"`
	Placement       string `json:"placement"`
	Name            string `json:"name"`
	CredentialScope string `json:"credentialScope"`
}

// Approval reports the two stages the portal grades separately. oddsock applies
// for a development account, so Dev is the one that decides whether a key arrives
// immediately; Ops describes what a later move to production would face and is
// surfaced because that is a decision a caller may need to make now.
//
// Absent on datasets whose page carries no 심의유형 row (LINK datasets, mostly) —
// unknown is reported as unknown rather than assumed to be automatic.
type Approval struct {
	Dev string `json:"dev,omitempty"` // 개발단계: 자동승인 | 심의승인
	Ops string `json:"ops,omitempty"` // 운영단계: 자동승인 | 심의승인
	Raw string `json:"raw,omitempty"` // the row verbatim, in case the wording changes
}

// AutoApproved reports whether applying yields a key without human review.
func (a *Approval) AutoApproved() bool {
	return a != nil && strings.Contains(a.Dev, "자동승인")
}

// reApproval reads "개발단계 : 자동승인 / 운영단계 : 심의승인". The
// redesigned page uses ASCII colons today, while older publisher fragments can
// contain the full-width form.
var reApproval = regexp.MustCompile(`개발단계\s*[:：]\s*(\S+)\s*/\s*운영단계\s*[:：]\s*(\S+)`)

// Operation is one 상세기능. When the request-variable table parses cleanly,
// Params is filled; otherwise RawHTML carries the section verbatim.
type Operation struct {
	Name         string       `json:"name"`
	Endpoint     string       `json:"endpoint,omitempty"`
	Params       []Param      `json:"params,omitempty"`
	RawHTML      string       `json:"rawHtml,omitempty"`
	ContractKind ContractKind `json:"contractKind,omitempty"`
}

// Param is one request variable, surfaced from the 요청변수 table.
type Param struct {
	Name     string `json:"name"`     // 항목명(영문)
	Required string `json:"required"` // 항목구분 (필수/옵션)
	Sample   string `json:"sample"`   // 샘플데이터
	Desc     string `json:"desc"`     // 항목설명
}

var reEndpoint = regexp.MustCompile(`https?://apis\.data\.go\.kr/[^\s"'<)]+`)

// reFileDownload pulls the two ids out of the 참고문서 link's
// fn_fileDownload('FILE_...','1') handler, which is the only place the page
// carries them.
var reFileDownload = regexp.MustCompile(`fn_fileDownload\('([^']+)'\s*,\s*'([^']+)'\)`)

// Describe scrapes {baseURL}/data/{pk}/openapi.do through the shared transport.
// baseURL is overridable for tests; production passes portal.BaseURL.
func Describe(ctx context.Context, f *fetch.Client, baseURL, pk string) (*APISpec, error) {
	if err := portal.ValidatePublicDataPK(pk); err != nil {
		return nil, err
	}
	base := strings.TrimRight(baseURL, "/") + "/data/" + pk
	url := base + "/openapi.do"
	doc, err := f.GetDoc(ctx, url)
	if err != nil {
		// The 2026 portal redesign can render an API+FILE dataset only at the
		// combined fileData route while the legacy openapi route returns 500.
		// Both are first-party detail pages for the same validated publicDataPk.
		url = base + "/fileData.do"
		doc, err = f.GetDoc(ctx, url)
		if err != nil {
			return nil, err
		}
	}

	spec := &APISpec{PublicDataPk: pk}
	spec.DataName = strings.TrimSpace(doc.Find(".h-tit, .open-api-title, .data-set-title").First().Text())
	spec.DataName = cleanText(spec.DataName)
	if spec.DataName == "" {
		if value, ok := labeledValue(doc.Selection, "OpenAPI 명"); ok {
			spec.DataName = cleanText(value.Text())
		}
	}
	// Decide the protocol branch before parsing any REST operation fragments.
	// A LINK page may contain REST-looking examples or stale selectors, but those
	// must neither trigger extra fragment requests nor become callable output.
	if value, ok := labeledValue(doc.Selection, "API 유형"); ok {
		spec.APIType = cleanText(value.Text())
	}
	preloadedOperations := operationsFromSwagger(doc)
	referencedSwaggerURL := ""
	if len(preloadedOperations) == 0 {
		if candidate := swaggerReferenceURL(doc, pk); candidate != "" {
			operations, swaggerErr := operationsFromSwaggerURL(ctx, f, candidate)
			if swaggerErr != nil {
				spec.Warnings = append(spec.Warnings, "공식 Swagger 문서를 읽지 못했습니다: "+swaggerErr.Error())
			} else if len(operations) > 0 {
				preloadedOperations = operations
				referencedSwaggerURL = candidate
			}
		}
	}
	// Combined API+FILE pages do not repeat the API 유형 label, but a validated
	// first-party Swagger document with callable operations is itself a REST
	// contract. This is an evidence-backed classification, not a title guess.
	if spec.APIType == "" && len(preloadedOperations) > 0 {
		spec.APIType = "REST"
	}

	if isRESTAPIType(spec.APIType) {
		// Operation containers: real per-operation content lives in
		// .open-api-detail-result (endpoint + 요청변수/출력결과 tables). The sibling
		// .open-api-detail div is only the operation-switcher (<select>+button, no
		// data) and, on pages with broken/comment-only-closed div nesting, can end
		// up as the *only* match for .open-api-detail while swallowing unrelated
		// content via the HTML5 parser's error recovery — so prefer the result
		// containers whenever the page has any, and only fall back to
		// .open-api-detail for older/simpler single-operation pages that lack a
		// separate result div.
		// The portal embeds an authoritative Swagger 2.0 spec on modern pages; prefer
		// it over scraping the rendered tables, which carry less and break more.
		if len(preloadedOperations) > 0 {
			spec.Operations = preloadedOperations
		}
		if len(spec.Operations) == 0 {
			ops, err := operationsFromKRDS(ctx, f, baseURL, doc, pk)
			if err != nil {
				return nil, err
			}
			spec.Operations = ops
		}

		sections := doc.Find(".open-api-detail-result")
		if sections.Length() == 0 {
			sections = doc.Find(".open-api-detail")
		}
		if len(spec.Operations) > 0 {
			sections = doc.Find("__none__") // swagger already answered; skip the tables
		}
		sections.Each(func(_ int, sel *goquery.Selection) {
			op := Operation{Name: cleanText(sel.Find("h4, .tit").First().Text()), ContractKind: ContractKindFirstPartyWebContract}
			if html, err := sel.Html(); err == nil {
				if m := reEndpoint.FindString(html); m != "" {
					op.Endpoint = m
				}
			}
			op.Params = parseParams(sel)
			if len(op.Params) == 0 {
				// surface-only: no clean request-variable table → hand back raw HTML.
				if html, err := sel.Html(); err == nil {
					op.RawHTML = strings.TrimSpace(html)
				}
			}
			if op.Name != "" || op.Endpoint != "" || op.RawHTML != "" {
				spec.Operations = append(spec.Operations, op)
			}
		})

		spec.Operations = dedupeOperations(spec.Operations)

		// Page-level endpoint fallback: many REST datasets carry the endpoint URL
		// somewhere on the page while documenting no 요청변수 table at all. Surfacing
		// the endpoint alone is still factual and gets a caller moving; the note below
		// makes clear the parameters are NOT documented here.
		if len(spec.Operations) == 0 {
			if html, err := doc.Html(); err == nil {
				if m := reEndpoint.FindString(html); m != "" {
					spec.Operations = append(spec.Operations, Operation{Endpoint: m, ContractKind: ContractKindFirstPartyWebContract})
					spec.EndpointOnly = true
				}
			}
		}
	}

	// Summary metadata used to be a th/td table. KRDS renders the same label/value
	// pairs as <strong class=key> + <div class=value>. Keep that markup choice in
	// one helper so every field gets the same compatibility boundary.
	if value, ok := labeledValue(doc.Selection, "심의유형"); ok {
		raw := cleanText(value.Text())
		a := &Approval{Raw: raw}
		if m := reApproval.FindStringSubmatch(raw); m != nil {
			a.Dev, a.Ops = m[1], m[2]
		}
		spec.Approval = a
	}

	// Legacy LINK pages render the publisher address in a URL row.
	var linkLookupErr error
	if value, ok := labeledValue(doc.Selection, "URL"); ok && isLinkAPIType(spec.APIType) {
		var raw string
		if href, found := value.Find("a[href]").First().Attr("href"); found {
			raw = href
		} else {
			raw = cleanText(value.Text())
		}
		if raw != "" {
			if err := spec.setExternalHandoff(raw); err != nil {
				linkLookupErr = err
			}
		}
	}
	// KRDS LINK pages render only a 바로가기 button. Its own JavaScript resolves
	// the publisher address through this JSON lookup when clicked. Keep that
	// portal detail hidden inside Describe: callers should not need a browser or
	// a second command merely because the portal changed how it renders the same
	// field.
	if isLinkAPIType(spec.APIType) && spec.LinkURL == "" {
		var resolved string
		resolved, linkLookupErr = resolvePortalLinkURL(ctx, f, baseURL, pk)
		if linkLookupErr == nil {
			linkLookupErr = spec.setExternalHandoff(resolved)
		}
		if linkLookupErr != nil && spec.Handoff == nil {
			spec.Handoff = failedExternalHandoff(linkLookupErr)
		}
	}

	// GuideDoc: the 참고문서 row. The file itself is never fetched or parsed here —
	// but its download URL is surfaced, because the file name alone gives an agent
	// nothing it can act on.
	guideValue, guideRowFound := labeledValue(doc.Selection, "참고문서")
	if guideRowFound {
		spec.GuideDoc = cleanText(guideValue.Text())
		if onclick, ok := guideValue.Find("a[onclick]").First().Attr("onclick"); ok {
			if m := reFileDownload.FindStringSubmatch(onclick); m != nil {
				spec.GuideDocURL = fmt.Sprintf("%s/cmm/cmm/fileDownload.do?atchFileId=%s&fileDetailSn=%s",
					strings.TrimRight(baseURL, "/"), m[1], m[2])
			}
		}
	}

	// Some datasets document the whole spec in the attached guide document and
	// leave the page itself empty. Say so, rather than handing back an empty list
	// that reads as "this API has no operations".
	if spec.EndpointOnly {
		spec.Note = "엔드포인트는 페이지에서 확인했지만 요청변수(파라미터) 표가 없습니다 — " +
			"파라미터는 포털에 문서화돼 있지 않습니다. guideDocUrl 이 있으면 그 문서를, 없으면 " +
			"제공기관 문서를 확인하세요. 파라미터를 추측해서 호출하지 마세요."
	} else if len(spec.Operations) == 0 {
		if isLinkAPIType(spec.APIType) {
			spec.Note = "이 API 는 유형이 LINK 입니다 — 포털은 명세를 싣지 않고 제공기관 사이트로 " +
				"연결만 합니다. 엔드포인트·파라미터는 포털에서 알 수 없으니 추측해서 호출하지 마세요."
			if spec.LinkURL != "" {
				spec.Note += " handoff.url(" + spec.LinkURL + ") 은 포털이 확인한 외부 시작점이며 API endpoint나 명세라고 단정할 수 없고 직접 호출하지 않습니다."
				if spec.Handoff != nil && spec.Handoff.Contract != nil {
					switch spec.Handoff.Contract.InvocationState {
					case InvocationImplemented:
						spec.Note += " 검증된 호출은 handoff.contract.operations에 있으며 provider key를 설정하면 call_api가 안전한 endpoint를 조립합니다."
					case InvocationBlockedInsecureTransport:
						spec.Note += " 제공기관 호출 endpoint가 HTTPS를 지원하지 않아 credential 자동 호출은 차단됩니다."
					default:
						spec.Note += " contract가 호출 가능으로 표시될 때까지 call_api로 보내지 마세요."
					}
				} else {
					spec.Note += " 계약을 검사하기 전에는 call_api로 호출할 수 없습니다."
				}
			} else if linkLookupErr != nil {
				spec.Note += " 포털의 제공기관 URL 조회도 실패했습니다: " + linkLookupErr.Error()
			}
		} else if isRESTAPIType(spec.APIType) {
			spec.Note = "이 페이지에는 상세기능·요청변수 표가 없습니다 — 명세가 참고문서(guideDocUrl)에만 있는 API입니다. " +
				"guideDocUrl 을 내려받아 읽고 엔드포인트·파라미터를 확인하세요. 파라미터를 추측해 호출하지 마세요."
		} else {
			spec.Note = "API 유형을 REST 또는 LINK로 확인하지 못했습니다 — 페이지 구조나 유형 표기가 바뀐 것일 수 있습니다. " +
				"oddsock doctor로 점검하고, 유형을 확인하기 전에는 엔드포인트를 추측하거나 call_api로 호출하지 마세요."
		}
		if isRESTAPIType(spec.APIType) && spec.GuideDocURL == "" {
			// A 참고문서 row that carries no file (fn_fileDownload('','')) is the
			// portal saying "no document" — distinct from the row being absent,
			// which would suggest the page layout changed.
			if guideRowFound {
				spec.Note = "이 API 는 포털에 명세가 없습니다 — 상세기능·요청변수 표도, 참고문서 파일도 " +
					"제공되지 않습니다(참고문서 항목이 비어 있음). 엔드포인트와 파라미터를 알 방법이 " +
					"포털에 없으니 제공기관에 문의하거나 다른 API 를 쓰세요. 추측해서 호출하지 마세요."
			} else {
				spec.Note = "이 페이지에서 상세기능·요청변수와 참고문서 항목 자체를 찾지 못했습니다 — " +
					"페이지 구조가 바뀐 것일 수 있습니다 (oddsock doctor 로 확인). 파라미터를 추측하지 마세요."
			}
		}
	}

	if referencedSwaggerURL != "" {
		spec.Evidence = append(spec.Evidence, SpecEvidence{
			Purpose: "operation-and-parameter-contract", Kind: "official_swagger",
			URL: referencedSwaggerURL, Stability: "documented",
		})
	}
	spec.Evidence = append(spec.Evidence, SpecEvidence{
		Purpose: "detail-and-required-parameters", Kind: "first_party_web_contract",
		URL: url, Stability: "fallback",
	})
	return spec, nil
}

// DescribeCatalogued starts from operation metadata already preserved in an
// official release snapshot, then uses the portal page only to fill facts the
// bulk API omits (notably required/sample parameter details and guide assets).
// A legacy/web snapshot simply follows the existing page contract.
func DescribeCatalogued(ctx context.Context, f *fetch.Client, baseURL, pk string) (*APISpec, error) {
	cat, err := catalog.Load()
	if err != nil {
		return Describe(ctx, f, baseURL, pk)
	}
	entry, ok := cat.Find(pk)
	if !ok || entry.OfficialAPI == nil {
		return Describe(ctx, f, baseURL, pk)
	}
	return DescribeCataloguedEntry(ctx, f, baseURL, entry)
}

// DescribeCataloguedEntry describes an entry already loaded by a higher-level
// catalogue workflow. It avoids reparsing the ~96k-entry snapshot for every
// API inspection while preserving the same official-first fallback contract.
func DescribeCataloguedEntry(ctx context.Context, f *fetch.Client, baseURL string, entry catalog.Entry) (*APISpec, error) {
	if entry.OfficialAPI == nil {
		return Describe(ctx, f, baseURL, entry.PK)
	}
	return describeWithOfficial(ctx, f, baseURL, entry)
}

func describeWithOfficial(ctx context.Context, f *fetch.Client, baseURL string, entry catalog.Entry) (*APISpec, error) {
	official := specFromOfficial(entry)
	fallback, err := Describe(ctx, f, baseURL, entry.PK)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		official.Warnings = append(official.Warnings, "포털 HTML fallback을 읽지 못해 공식 목록 API가 제공한 범위만 반환합니다: "+err.Error())
		return official, nil
	}
	return mergeOfficialSpec(official, fallback), nil
}

func specFromOfficial(entry catalog.Entry) *APISpec {
	contract := entry.OfficialAPI
	spec := &APISpec{PublicDataPk: entry.PK, DataName: entry.Title, OfficialAPI: contract}
	if contract == nil {
		return spec
	}
	spec.APIType = contract.APIType
	if contract.DevApproval != "" || contract.ProdApproval != "" {
		spec.Approval = &Approval{Dev: contract.DevApproval, Ops: contract.ProdApproval,
			Raw: "개발단계 : " + contract.DevApproval + " / 운영단계 : " + contract.ProdApproval}
	}
	for _, source := range contract.Operations {
		op := Operation{Name: source.Name, Endpoint: source.URL, ContractKind: ContractKind(contract.EvidenceKind)}
		if op.ContractKind == "" {
			op.ContractKind = ContractKindOfficialAPI
		}
		for _, name := range source.RequestNames {
			op.Params = append(op.Params, Param{Name: name, Required: ParamRequirementUnknown, Desc: "공식 목록 API는 필수 여부·샘플을 제공하지 않음"})
		}
		if op.Name != "" || op.Endpoint != "" || len(op.Params) > 0 {
			spec.Operations = append(spec.Operations, op)
		}
	}
	if isLinkAPIType(spec.APIType) && strings.TrimSpace(contract.LinkURL) != "" {
		if err := spec.setExternalHandoff(contract.LinkURL); err != nil {
			spec.Warnings = append(spec.Warnings, "공식 목록 API의 LINK URL을 안전한 handoff로 해석하지 못했습니다: "+err.Error())
		}
	}
	spec.Note = "공식 목록 API가 operation·요청변수 이름을 제공했지만 필수 여부·샘플 값은 제공하지 않습니다. evidence의 HTML fallback이 그 세부 계약을 보완합니다."
	kind, evidenceURL := contract.EvidenceKind, contract.EvidenceURL
	if kind == "" {
		kind = "official_api"
	}
	if evidenceURL == "" {
		evidenceURL = "https://api.odcloud.kr/api/15077093/v1/open-data-list"
	}
	spec.Evidence = []SpecEvidence{{
		Purpose: "dataset-operation-metadata", Kind: kind,
		URL: evidenceURL, Stability: "documented",
	}}
	return spec
}

func mergeOfficialSpec(official, fallback *APISpec) *APISpec {
	if fallback == nil {
		return official
	}
	fallback.OfficialAPI = official.OfficialAPI
	fallback.Evidence = append(append([]SpecEvidence(nil), official.Evidence...), fallback.Evidence...)
	fallback.Warnings = append(append([]string(nil), official.Warnings...), fallback.Warnings...)
	if official.DataName != "" {
		fallback.DataName = official.DataName
	}
	if official.APIType != "" {
		fallback.APIType = official.APIType
	}
	if official.Approval != nil {
		fallback.Approval = official.Approval
	}
	if official.LinkURL != "" {
		fallback.LinkURL, fallback.Handoff = official.LinkURL, official.Handoff
	}
	fallback.Operations = mergeSpecOperations(fallback.Operations, official.Operations)
	if len(official.Operations) > 0 {
		fallback.Note = official.Note
	}
	return fallback
}

func mergeSpecOperations(detailed, official []Operation) []Operation {
	out := append([]Operation(nil), detailed...)
	for _, source := range official {
		matched := -1
		for index := range out {
			if source.Name != "" && strings.EqualFold(strings.TrimSpace(out[index].Name), strings.TrimSpace(source.Name)) {
				matched = index
				break
			}
		}
		if matched < 0 {
			out = append(out, source)
			continue
		}
		if source.Endpoint != "" {
			out[matched].Endpoint = source.Endpoint
		}
		have := map[string]bool{}
		for _, param := range out[matched].Params {
			have[strings.ToLower(strings.TrimSpace(param.Name))] = true
		}
		for _, param := range source.Params {
			if name := strings.ToLower(strings.TrimSpace(param.Name)); name != "" && !have[name] {
				out[matched].Params = append(out[matched].Params, param)
				have[name] = true
			}
		}
	}
	return dedupeOperations(out)
}

func isLinkAPIType(apiType string) bool {
	return strings.Contains(strings.ToUpper(apiType), "LINK")
}

func isRESTAPIType(apiType string) bool {
	return strings.Contains(strings.ToUpper(apiType), "REST")
}

// ValidateDataGoKRApplication keeps LINK datasets out of the portal's REST
// application form. Known external providers surface their own application
// URL; unknown LINK providers surface the official handoff for inspection.
func ValidateDataGoKRApplication(spec *APISpec) error {
	if spec == nil {
		return fmt.Errorf("활용신청 전에 dataset 명세가 필요합니다")
	}
	if isLinkAPIType(spec.APIType) {
		target := spec.LinkURL
		if spec.Handoff != nil && spec.Handoff.Contract != nil && spec.Handoff.Contract.ApplicationURL != "" {
			target = spec.Handoff.Contract.ApplicationURL
		}
		if target == "" {
			target = "inspect_dataset의 handoff.nextAction"
		}
		return fmt.Errorf("pk=%s 는 LINK 유형이라 data.go.kr 활용신청 대상이 아닙니다 — 제공기관 신청/안내: %s", spec.PublicDataPk, target)
	}
	if isRESTAPIType(spec.APIType) {
		return nil
	}
	return fmt.Errorf("pk=%s 의 API 유형을 REST로 확인할 수 없어 data.go.kr 활용신청을 중단합니다", spec.PublicDataPk)
}

func (s *APISpec) setExternalHandoff(raw string) error {
	validated, err := validatePublisherLinkURL(raw)
	if err != nil {
		return err
	}
	u, _ := url.Parse(validated) // validatePublisherLinkURL already proved it parses.
	s.LinkURL = validated
	s.Handoff = &ExternalHandoff{
		URL:         validated,
		Host:        strings.TrimSuffix(strings.ToLower(u.Hostname()), "."),
		Trust:       HandoffPublisherUntrusted,
		FetchPolicy: HandoffSafeFetcherRequired,
		State:       HandoffInspectionRequired,
		NextAction:  HandoffInspectContract,
	}
	if contract := knownExternalContract(u); contract != nil {
		s.Handoff.State = HandoffContractKnown
		s.Handoff.Contract = contract
		switch contract.InvocationState {
		case InvocationImplemented:
			s.Handoff.NextAction = HandoffRequestAccess
		case InvocationNotImplemented:
			s.Handoff.NextAction = HandoffUseProviderDirectly
		default:
			s.Handoff.NextAction = HandoffChooseAnother
		}
	}
	return nil
}

func failedExternalHandoff(err error) *ExternalHandoff {
	failure := &HandoffFailure{Code: "link_resolution_failed", Message: err.Error()}
	nextAction := HandoffChooseAnother
	var resolutionErr *linkResolutionError
	if errors.As(err, &resolutionErr) {
		failure.Code = resolutionErr.code
		failure.Retryable = resolutionErr.retryable
		if resolutionErr.retryable {
			nextAction = HandoffRetryResolution
		}
	}
	return &ExternalHandoff{
		State:      HandoffResolutionFailed,
		NextAction: nextAction,
		Failure:    failure,
	}
}

// resolvePortalLinkURL mirrors the portal's fn_goUrlLink click without driving
// a browser. The endpoint belongs to data.go.kr and returns the publisher URL;
// it is not the publisher API itself and needs no external credentials.
func resolvePortalLinkURL(ctx context.Context, f *fetch.Client, baseURL, pk string) (string, error) {
	u, err := url.Parse(strings.TrimRight(baseURL, "/") + "/tcs/dss/selectApiLinkUrl.do")
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("publicDataPk", pk)
	u.RawQuery = q.Encode()

	res, err := f.GetNoRedirect(ctx, u.String())
	if err != nil {
		return "", &linkResolutionError{code: "link_transport_error", retryable: true, err: err}
	}
	if res.Status != 200 {
		return "", &linkResolutionError{
			code: "link_http_error", retryable: res.Status == 429 || res.Status >= 500,
			err: fmt.Errorf("LINK 주소 조회 HTTP %d", res.Status),
		}
	}
	var payload struct {
		Status  bool   `json:"status"`
		LinkURL string `json:"linkUrl"`
		Error   string `json:"errorDc"`
	}
	if err := json.Unmarshal(res.Body, &payload); err != nil {
		return "", &linkResolutionError{code: "link_invalid_response", err: fmt.Errorf("LINK 주소 응답 해석 실패: %w", err)}
	}
	if !payload.Status {
		if payload.Error == "" {
			payload.Error = "포털이 실패 상태를 반환했습니다"
		}
		return "", &linkResolutionError{code: "link_portal_rejected", err: fmt.Errorf("LINK 주소 조회 실패: %s", cleanText(payload.Error))}
	}
	validated, err := validatePublisherLinkURL(payload.LinkURL)
	if err != nil {
		return "", &linkResolutionError{code: "link_unsafe_target", err: err}
	}
	return validated, nil
}

func validatePublisherLinkURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil ||
		(!strings.EqualFold(u.Scheme, "https") && !strings.EqualFold(u.Scheme, "http")) {
		return "", fmt.Errorf("포털이 유효한 HTTP(S) 제공기관 URL을 반환하지 않았습니다")
	}
	host, err := normalizePublisherURLHost(u)
	if err != nil {
		return "", fmt.Errorf("포털이 유효한 제공기관 호스트를 반환하지 않았습니다")
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || host == "local" || strings.HasSuffix(host, ".local") {
		return "", fmt.Errorf("포털이 외부에서 안전하게 검사할 수 없는 제공기관 주소를 반환했습니다")
	}
	if strings.Contains(host, "%") || looksLikeObscureNumericHost(host) {
		return "", fmt.Errorf("포털이 외부에서 안전하게 검사할 수 없는 제공기관 주소를 반환했습니다")
	}
	if ip := net.ParseIP(host); ip != nil &&
		(ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast()) {
		return "", fmt.Errorf("포털이 외부에서 안전하게 검사할 수 없는 제공기관 주소를 반환했습니다")
	}
	return u.String(), nil
}

func normalizePublisherURLHost(u *url.URL) (string, error) {
	host := strings.TrimSuffix(u.Hostname(), ".")
	for _, r := range host {
		if r > 127 {
			var err error
			host, err = idna.Lookup.ToASCII(host)
			if err != nil {
				return "", err
			}
			break
		}
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "" {
		return "", fmt.Errorf("empty host")
	}
	if port := u.Port(); port != "" {
		u.Host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		u.Host = "[" + host + "]"
	} else {
		u.Host = host
	}
	return host, nil
}

func looksLikeObscureNumericHost(host string) bool {
	if host == "" {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" {
			return false
		}
		if strings.HasPrefix(label, "0x") {
			if len(label) == 2 {
				return false
			}
			for _, r := range label[2:] {
				if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
					return false
				}
			}
			continue
		}
		for _, r := range label {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

// operationsFromKRDS reads the operation fragment rendered in the detail page,
// then asks the portal's own fragment endpoint for every remaining select
// option. The endpoint returns HTML rather than JSON, but it is still the
// narrowest stable boundary: no browser, clicks, or timing assumptions.
func operationsFromKRDS(ctx context.Context, f *fetch.Client, baseURL string, doc *goquery.Document, pk string) ([]Operation, error) {
	type option struct {
		seq  string
		name string
	}
	var options []option
	doc.Find("#open_api_detail_select option[value]").Each(func(_ int, sel *goquery.Selection) {
		seq, _ := sel.Attr("value")
		if seq = strings.TrimSpace(seq); seq != "" {
			options = append(options, option{seq: seq, name: cleanText(sel.Text())})
		}
	})

	root := doc.Find("#apiDetailFunctionDiv").First()
	// The legacy page reused this wrapper around .open-api-detail-result. Its
	// operation parser below already handles that shape; only the KRDS fragment
	// carries .data-report-group and uses the on-demand endpoint described here.
	if root.Length() == 0 || root.Find(".data-report-group").Length() == 0 {
		return nil, nil
	}

	var ops []Operation
	next := 0
	if root.Length() > 0 {
		name := ""
		if len(options) > 0 {
			name = options[0].name // the select's first option is rendered initially
			next = 1
		}
		if op := operationFromKRDS(root, name); operationHasContent(op) {
			ops = append(ops, op)
		} else {
			next = 0 // initial fragment was empty; fetch every named option explicitly
		}
	}
	if next >= len(options) {
		return ops, nil
	}

	detailPK, _ := doc.Find("#publicDataDetailPk").First().Attr("value")
	pagePK, _ := doc.Find("#publicDataPk").First().Attr("value")
	if pagePK == "" {
		pagePK = pk
	}
	if detailPK == "" {
		return nil, fmt.Errorf("pk=%s 의 상세기능 선택지는 있지만 publicDataDetailPk 가 없습니다 — 페이지 구조가 바뀌었을 수 있습니다", pk)
	}

	fragmentURL := strings.TrimRight(baseURL, "/") + "/tcs/dss/selectApiDetailFunction.do"
	for _, item := range options[next:] {
		fragment, err := f.PostFormDoc(ctx, fragmentURL, url.Values{
			"oprtinSeqNo":        {item.seq},
			"publicDataDetailPk": {detailPK},
			"publicDataPk":       {pagePK},
		})
		if err != nil {
			return nil, fmt.Errorf("pk=%s 상세기능 %q 조회 실패: %w", pk, item.name, err)
		}
		op := operationFromKRDS(fragment.Selection, item.name)
		if !operationHasContent(op) {
			return nil, fmt.Errorf("pk=%s 상세기능 %q 응답에서 엔드포인트·요청변수를 찾지 못했습니다 — 페이지 구조가 바뀌었을 수 있습니다", pk, item.name)
		}
		ops = append(ops, op)
	}
	return ops, nil
}

func operationFromKRDS(root *goquery.Selection, name string) Operation {
	op := Operation{Name: cleanText(name), ContractKind: ContractKindFirstPartyWebContract}
	if value, ok := labeledValue(root, "요청주소"); ok {
		op.Endpoint = reEndpoint.FindString(value.Text())
	}
	op.Params = parseParams(root)
	if len(op.Params) == 0 {
		if html, err := root.Html(); err == nil {
			op.RawHTML = strings.TrimSpace(html)
		}
	}
	return op
}

func operationHasContent(op Operation) bool {
	return op.Endpoint != "" || len(op.Params) > 0 || op.RawHTML != ""
}

// labeledValue returns the value beside a portal summary label across both
// layouts observed in production: the legacy th/td table and the KRDS
// key/value list. A caller asks in domain terms ("API 유형"), not markup terms.
func labeledValue(root *goquery.Selection, label string) (*goquery.Selection, bool) {
	matches := func(text string) bool {
		text = cleanText(text)
		if label == "URL" { // avoid matching labels that merely contain "URL"
			return text == label
		}
		return strings.Contains(text, label)
	}

	var value *goquery.Selection
	root.Find("th").EachWithBreak(func(_ int, th *goquery.Selection) bool {
		if !matches(th.Text()) {
			return true
		}
		candidate := th.NextFiltered("td").First()
		if candidate.Length() > 0 {
			value = candidate
			return false
		}
		return true
	})
	if value != nil {
		return value, true
	}

	root.Find(".key").EachWithBreak(func(_ int, key *goquery.Selection) bool {
		if !matches(key.Text()) {
			return true
		}
		candidate := key.Parent().ChildrenFiltered(".value").First()
		if candidate.Length() == 0 {
			candidate = key.NextFiltered(".value").First()
		}
		if candidate.Length() > 0 {
			value = candidate
			return false
		}
		return true
	})
	return value, value != nil
}

// parseParams reads the 요청변수 (request parameter) table inside an operation
// section. 요청변수 and 출력결과 (response) tables share an identical header row
// (항목명(영문), ...), so column-header matching alone can't tell them apart —
// instead this walks headings and tables in document order and only parses a
// table anchored under the nearest preceding 요청변수/Request Parameter
// heading, never under 출력결과/Response. Response fields (resultCode,
// resultMsg, ...) can therefore never be misattributed as request params. If
// no heading unambiguously marks a table as the request table, it's left to
// RawHTML rather than guessed.
func parseParams(sel *goquery.Selection) []Param {
	var params []Param
	heading := ""
	sel.Find("h4, table").EachWithBreak(func(_ int, node *goquery.Selection) bool {
		if goquery.NodeName(node) == "h4" {
			heading = cleanText(node.Text())
			return true
		}
		if !isRequestHeading(heading) {
			return true // not under a 요청변수 heading (or under 출력결과); keep scanning
		}
		tbl := node
		headers := map[string]int{}
		tbl.Find("thead th, tr:first-child th").Each(func(i int, th *goquery.Selection) {
			headers[cleanText(th.Text())] = i
		})
		nameCol, ok := colIndex(headers, "항목명(영문)")
		if !ok {
			return true // heading said request, but table shape is unrecognized; keep scanning
		}
		reqCol, _ := colIndex(headers, "항목구분")
		sampleCol, _ := colIndex(headers, "샘플데이터")
		descCol, _ := colIndex(headers, "항목설명")
		tbl.Find("tbody tr").Each(func(_ int, tr *goquery.Selection) {
			cells := tr.Find("td")
			if cells.Length() == 0 {
				return
			}
			get := func(idx int) string {
				if idx < 0 {
					return ""
				}
				return cleanText(cells.Eq(idx).Text())
			}
			name := get(nameCol)
			if name == "" {
				return
			}
			params = append(params, Param{
				Name:     name,
				Required: get(reqCol),
				Sample:   get(sampleCol),
				Desc:     get(descCol),
			})
		})
		return false // took the first request-variable table
	})
	return params
}

// isRequestHeading reports whether a heading text marks the following table
// as a 요청변수(Request Parameter) table rather than 출력결과(Response Element).
func isRequestHeading(h string) bool {
	if h == "" {
		return false
	}
	lower := strings.ToLower(h)
	if strings.Contains(h, "출력결과") || strings.Contains(lower, "response") {
		return false
	}
	return strings.Contains(h, "요청변수") || strings.Contains(lower, "request parameter")
}

func colIndex(headers map[string]int, key string) (int, bool) {
	i, ok := headers[key]
	if !ok {
		return -1, false
	}
	return i, true
}

func cleanText(s string) string { return strings.Join(strings.Fields(s), " ") }

// reSwaggerJSON pulls the embedded spec out of the page's
// `var swaggerJson = \`{…}\`;` template literal.
var reSwaggerJSON = regexp.MustCompile("(?s)swaggerJson\\s*=\\s*`(.*?)`")

var reSwaggerReference = regexp.MustCompile(`(?i)url\s*:\s*['"](https://infuser\.odcloud\.kr/oas/docs\?namespace=([0-9]+)/v[0-9]+)['"]`)

// swaggerDoc is the slice of Swagger 2.0 oddsock reads. Parameters sit at the
// PATH level on data.go.kr's specs, not under the operation, so both are read.
type swaggerDoc struct {
	Host     string   `json:"host"`
	BasePath string   `json:"basePath"`
	Schemes  []string `json:"schemes"`
	Paths    map[string]struct {
		Parameters []swaggerParam `json:"parameters"`
		Get        *swaggerOp     `json:"get"`
		Post       *swaggerOp     `json:"post"`
	} `json:"paths"`
}

type swaggerOp struct {
	Summary     string         `json:"summary"`
	Description string         `json:"description"`
	OperationID string         `json:"operationId"`
	Parameters  []swaggerParam `json:"parameters"`
}

type swaggerParam struct {
	Name        string `json:"name"`
	In          string `json:"in"`
	Required    bool   `json:"required"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Example     any    `json:"example"`
}

// operationsFromSwagger reads the embedded Swagger spec, if the page carries one.
// This is the authoritative source — the HTML tables are a fallback for pages
// that predate it. Returns nil when there is no spec to read.
func operationsFromSwagger(doc *goquery.Document) []Operation {
	var raw string
	doc.Find("script").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		if m := reSwaggerJSON.FindStringSubmatch(s.Text()); m != nil {
			raw = m[1]
			return false
		}
		return true
	})
	if raw == "" {
		return nil
	}
	return operationsFromSwaggerJSON([]byte(raw))
}

func swaggerReferenceURL(doc *goquery.Document, pk string) string {
	var found string
	doc.Find("script").EachWithBreak(func(_ int, script *goquery.Selection) bool {
		match := reSwaggerReference.FindStringSubmatch(script.Text())
		if len(match) == 3 && match[2] == pk {
			found = match[1]
			return false
		}
		return true
	})
	return found
}

func operationsFromSwaggerURL(ctx context.Context, client *fetch.Client, rawURL string) ([]Operation, error) {
	response, err := client.Get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	if response.Status != 200 {
		return nil, fmt.Errorf("GET %s: unexpected status %d", rawURL, response.Status)
	}
	operations := operationsFromSwaggerJSON(response.Body)
	if len(operations) == 0 {
		return nil, fmt.Errorf("GET %s: Swagger operation을 찾지 못했습니다", rawURL)
	}
	return operations, nil
}

func operationsFromSwaggerJSON(raw []byte) []Operation {
	var sd swaggerDoc
	if err := json.Unmarshal(raw, &sd); err != nil || sd.Host == "" {
		return nil
	}
	scheme := "https"
	for _, s := range sd.Schemes {
		if s == "https" {
			scheme = s
			break
		}
		scheme = s
	}
	var ops []Operation
	for path, item := range sd.Paths {
		op := item.Get
		if op == nil {
			op = item.Post
		}
		if op == nil {
			continue
		}
		name := op.Summary
		if name == "" {
			name = op.OperationID
		}
		// Path-level parameters first, then any the operation adds.
		params := append(append([]swaggerParam{}, item.Parameters...), op.Parameters...)
		o := Operation{
			Name:         cleanText(name),
			Endpoint:     scheme + "://" + strings.TrimRight(sd.Host, "/") + sd.BasePath + path,
			ContractKind: ContractKindOfficialSwagger,
		}
		for _, p := range params {
			req := "옵션"
			if p.Required {
				req = "필수"
			}
			sample := ""
			if p.Example != nil {
				sample = cleanText(fmt.Sprint(p.Example))
			}
			o.Params = append(o.Params, Param{
				Name:     p.Name,
				Required: req,
				Sample:   sample,
				Desc:     cleanText(p.Description),
			})
		}
		ops = append(ops, o)
	}
	sort.Slice(ops, func(i, j int) bool { return ops[i].Endpoint < ops[j].Endpoint })
	return ops
}

// dedupeOperations drops operations that are byte-identical to one already kept.
// Some portal pages render the same 상세기능 block twice — a PC table and a mobile
// one, distinguished only by presentation classes — and counting both makes a
// one-operation dataset look like two, which then reads as an ambiguous choice a
// caller has no way to resolve (both alternatives are the same call).
//
// Only exact duplicates are collapsed: same name, same endpoint, same parameter
// names in the same order. Two operations that share an endpoint but document
// different variables are genuinely different and are both kept.
func dedupeOperations(ops []Operation) []Operation {
	if len(ops) < 2 {
		return ops
	}
	seen := make(map[string]bool, len(ops))
	out := make([]Operation, 0, len(ops))
	for _, op := range ops {
		names := make([]string, 0, len(op.Params))
		for _, p := range op.Params {
			names = append(names, p.Name)
		}
		sig := op.Name + "\x00" + op.Endpoint + "\x00" + strings.Join(names, ",")
		if seen[sig] {
			continue
		}
		seen[sig] = true
		out = append(out, op)
	}
	return out
}
