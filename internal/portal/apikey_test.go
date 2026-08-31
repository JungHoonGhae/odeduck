package portal

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// The page lists every key ever issued; only the hidden input marks the active
// one. Parsing must take that, not the first key it finds in the table.
func TestParseAPIKey(t *testing.T) {
	body, err := os.ReadFile("testdata/apikeylist.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	key, err := parseAPIKey(string(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !strings.HasSuffix(key, "active") {
		t.Errorf("key = %q, want the active (hidden-input) key, not a stale table row", key)
	}
}

// A page without the key (no approved application yet, or markup drift) must
// error rather than hand back an empty key that would fail confusingly later.
func TestParseAPIKeyMissing(t *testing.T) {
	if _, err := parseAPIKey(`<html><body><p>인증키 없음</p></body></html>`); !errors.Is(err, ErrAPIKeyFieldMissing) {
		t.Errorf("missing field error = %v, want ErrAPIKeyFieldMissing", err)
	}
	if _, err := parseAPIKey(`<input id="pblisrCrtfcKeyPlain" value="">`); !errors.Is(err, ErrAPIKeyNotIssued) {
		t.Errorf("empty field error = %v, want ErrAPIKeyNotIssued", err)
	}
}

// An expired session can redirect the key URL to the generic portal homepage.
// That is an authentication problem, not evidence that the key-page markup
// drifted. Conversely, a page that still identifies itself as the key list but
// lost the active-key field is genuine drift and must remain a hard failure.
func TestAPIKeyFromAccountPageDistinguishesLogoutFromMarkupDrift(t *testing.T) {
	loggedOut := `<html><head><title>공공데이터포털</title></head><body><main>공공데이터 검색</main></body></html>`
	if _, err := apiKeyFromAccountPage(loggedOut); !errors.Is(err, ErrNotLoggedIn) {
		t.Errorf("generic portal page error = %v, want ErrNotLoggedIn", err)
	}

	drifted := `<html><body><h3>인증키 발급현황</h3><table><tr><th>인증키</th></tr></table></body></html>`
	if _, err := apiKeyFromAccountPage(drifted); !errors.Is(err, ErrAPIKeyFieldMissing) {
		t.Errorf("identified key page without field error = %v, want ErrAPIKeyFieldMissing", err)
	}
}
