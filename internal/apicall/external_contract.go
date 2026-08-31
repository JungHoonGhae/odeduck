package apicall

import (
	"net/url"
	"strings"
)

// knownExternalContract is deliberately a narrow, evidence-backed registry.
// A provider URL pattern is added only after its official application and
// authentication documents have been inspected. An otherwise known host at an
// unverified path retains the generic inspection_required handoff; we never
// infer a provider contract from hostname or URL words alone.
func knownExternalContract(target *url.URL) *ExternalContract {
	host := strings.TrimSuffix(strings.ToLower(target.Hostname()), ".")
	path := strings.TrimSuffix(target.EscapedPath(), "/")
	secureOrigin := strings.EqualFold(target.Scheme, "https") && (target.Port() == "" || target.Port() == "443")
	switch {
	case secureOrigin && target.RawQuery == "" && target.Fragment == "" &&
		(host == "safetykorea.kr" || host == "www.safetykorea.kr") && path == "/release/openapi":
		return &ExternalContract{
			Provider:             "SafetyKorea",
			DocumentationURL:     "https://www.safetykorea.kr/resources/fileData/openapi/Open_API_%EC%82%AC%EC%9A%A9%EC%84%A4%EB%AA%85%EC%84%9C_v2.0.pdf",
			DocumentationVersion: "2.0 (2025-06-30)",
			ApplicationURL:       "https://www.safetykorea.kr/release/openapi2",
			AccessMode:           "manual_approval",
			Auth: &ExternalAuthContract{
				Type:            "api_key",
				Placement:       "header",
				Name:            "AuthKey",
				CredentialScope: "safetykorea.kr",
			},
			InvocationState: "not_implemented",
			VerifiedAt:      "2026-09-01",
		}
	default:
		return nil
	}
}
