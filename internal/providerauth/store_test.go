package providerauth

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func isolateConfigHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
}

func TestCredentialLifecycleIsScopedAndNeverListed(t *testing.T) {
	isolateConfigHome(t)
	const secret = "SAFETY-SECRET-123"
	if err := Set("safetykorea", Credential{Key: secret}); err != nil {
		t.Fatalf("set: %v", err)
	}

	got, err := Get("safetykorea", "https://www.safetykorea.kr/openapi/api/")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Key != secret || got.Domain != "" {
		t.Fatalf("credential = %#v", got)
	}
	if _, err := Get("safetykorea", "https://evil.example/"); err == nil || !strings.Contains(err.Error(), "scope") {
		t.Fatalf("wrong scope error = %v", err)
	}

	statuses, err := Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 3 {
		t.Fatalf("statuses = %#v", statuses)
	}
	for _, status := range statuses {
		if strings.Contains(status.Provider+status.Scope+status.Domain, secret) {
			t.Fatalf("status leaked secret: %#v", status)
		}
	}
	if !statuses[0].Configured || statuses[0].Provider != "safetykorea" {
		t.Fatalf("safety status = %#v", statuses[0])
	}

	path, err := credentialPath("safetykorea")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows reports synthesized POSIX mode bits; its real boundary is the
	// protected DACL covered by store_windows_test.go.
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("credential mode = %o, want 600", info.Mode().Perm())
	}

	if err := Delete("safetykorea"); err != nil {
		t.Fatal(err)
	}
	if _, err := Get("safetykorea", "https://www.safetykorea.kr/openapi/api/"); err == nil {
		t.Fatal("deleted credential remained readable")
	}
}

func TestStatusAndDeleteDoNotCreateProviderCredentialDirectory(t *testing.T) {
	isolateConfigHome(t)
	if _, err := Status(); err != nil {
		t.Fatal(err)
	}
	if err := Delete("safetykorea"); err != nil {
		t.Fatal(err)
	}
	dir, err := credentialDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("read-only credential commands created %s: %v", dir, err)
	}
}

func TestVWorldCredentialKeepsValidatedPublicDomainMetadata(t *testing.T) {
	isolateConfigHome(t)
	cred := Credential{Key: "VWORLD-SECRET-123", Domain: "https://example.com/map"}
	if err := Set("vworld", cred); err != nil {
		t.Fatal(err)
	}
	got, err := Get("vworld", "https://api.vworld.kr/req/")
	if err != nil {
		t.Fatal(err)
	}
	if got != cred {
		t.Fatalf("got %#v, want %#v", got, cred)
	}
	for _, invalid := range []string{"javascript:alert(1)", "https://user@example.com/", "https://example.com/#secret"} {
		if err := Set("vworld", Credential{Key: "VWORLD-SECRET-123", Domain: invalid}); err == nil {
			t.Errorf("accepted invalid domain %q", invalid)
		}
	}
}

func TestCredentialStoreRejectsUnknownProviderAndWeakInput(t *testing.T) {
	isolateConfigHome(t)
	for _, tc := range []struct {
		provider string
		cred     Credential
	}{
		{"unknown", Credential{Key: "LONG-ENOUGH-SECRET"}},
		{"foodsafetykorea", Credential{Key: "short"}},
		{"foodsafetykorea", Credential{Key: "LINE1\nLINE2"}},
		{"safetykorea", Credential{Key: "LONG-ENOUGH", Domain: "https://example.com"}},
	} {
		if err := Set(tc.provider, tc.cred); err == nil {
			t.Errorf("Set(%q, %#v) succeeded", tc.provider, tc.cred)
		}
	}
}

func TestProviderDefinitionsExposeExactInvocationSupport(t *testing.T) {
	if got := strings.Join(ProviderIDs(), ","); got != "safetykorea,foodsafetykorea,vworld" {
		t.Fatalf("provider IDs = %q", got)
	}
	if !Supports("vworld", "https://api.vworld.kr/req/") {
		t.Fatal("VWorld exact credential scope should be supported")
	}
	if Supports("vworld", "https://api.vworld.kr/") || Supports("seoul-open-data", "http://openapi.seoul.go.kr:8088/") {
		t.Fatal("broad or unstored provider scope was accepted")
	}
}

func TestClearAllRemovesEveryProviderCredential(t *testing.T) {
	isolateConfigHome(t)
	for _, provider := range []string{"safetykorea", "foodsafetykorea", "vworld"} {
		if err := Set(provider, Credential{Key: "SECRET-FOR-" + provider}); err != nil {
			t.Fatal(err)
		}
	}
	dir, err := credentialDir()
	if err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(dir, ".provider-key-interrupted")
	if err := os.WriteFile(orphan, []byte("PARTIAL-SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ClearAll(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("interrupted credential temp remained after ClearAll: %v", err)
	}
	statuses, err := Status()
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range statuses {
		if status.Configured {
			t.Errorf("credential remained configured: %#v", status)
		}
	}
}

func TestClearAllRemovesCredentialsFromCurrentAndLegacyConfigRoots(t *testing.T) {
	isolateConfigHome(t)
	configHome, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{
		filepath.Join(configHome, "oddsock", "provider-credentials", "safetykorea.json"),
		filepath.Join(configHome, "opendatactl", "provider-credentials", "safetykorea.json"),
		filepath.Join(configHome, "gongctl", "provider-credentials", "safetykorea.json"),
	}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(`{"key":"SAFETY-SECRET-123"}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := ClearAll(); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("credential remained after ClearAll: %s (%v)", path, err)
		}
	}
}

func TestClearAllContinuesAfterOneProviderDeleteFails(t *testing.T) {
	isolateConfigHome(t)
	for _, provider := range []string{"safetykorea", "foodsafetykorea", "vworld"} {
		if err := Set(provider, Credential{Key: "SECRET-FOR-" + provider}); err != nil {
			t.Fatal(err)
		}
	}

	blocked, err := credentialPath("safetykorea")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(blocked); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(blocked, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blocked, "keep"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ClearAll(); err == nil {
		t.Fatal("ClearAll should report the blocked safetykorea path")
	}
	for _, provider := range []string{"foodsafetykorea", "vworld"} {
		def, ok := definition(provider)
		if !ok {
			t.Fatalf("missing provider definition: %s", provider)
		}
		if _, err := Get(provider, def.Scope); !errors.Is(err, ErrNotConfigured) {
			t.Fatalf("%s credential was not cleared after an earlier failure: %v", provider, err)
		}
	}
}
