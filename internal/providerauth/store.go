// Package providerauth stores credentials issued by external LINK providers.
// Values never enter catalogue/spec output or MCP arguments: callers identify a
// reviewed adapter and scope, and this package returns only the matching secret.
package providerauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/JungHoonGhae/odeduck/internal/portal"
)

type providerDefinition struct {
	ID          string
	Scope       string
	AllowDomain bool
}

var providerDefinitions = []providerDefinition{
	{ID: "safetykorea", Scope: "https://www.safetykorea.kr/openapi/api/"},
	{ID: "foodsafetykorea", Scope: "https://openapi.foodsafetykorea.go.kr/api/"},
	{ID: "vworld", Scope: "https://api.vworld.kr/req/", AllowDomain: true},
}

// Credential is the provider-issued key plus optional public registration
// metadata. Domain is not a secret, but it lives beside the key so a VWorld key
// can never accidentally be used with another key's registered domain.
type Credential struct {
	Key    string `json:"key"`
	Domain string `json:"domain,omitempty"`
}

// ProviderStatus deliberately omits the credential value.
type ProviderStatus struct {
	Provider   string `json:"provider"`
	Scope      string `json:"scope"`
	Configured bool   `json:"configured"`
	Domain     string `json:"domain,omitempty"`
}

var ErrNotConfigured = errors.New("provider credential이 설정되지 않았습니다")

func definition(provider string) (providerDefinition, bool) {
	provider = strings.TrimSpace(provider)
	for _, candidate := range providerDefinitions {
		if candidate.ID == provider {
			return candidate, true
		}
	}
	return providerDefinition{}, false
}

// ProviderIDs returns the fixed credential namespaces understood by the local
// store. CLI help and adapter diagnostics use this inventory so a new invoker
// cannot silently advertise a provider whose key can never be configured.
func ProviderIDs() []string {
	ids := make([]string, 0, len(providerDefinitions))
	for _, def := range providerDefinitions {
		ids = append(ids, def.ID)
	}
	return ids
}

// Supports is the fail-closed seam between an adapter contract and the local
// credential store. Both provider ID and exact scope must be registered.
func Supports(provider, scope string) bool {
	def, ok := definition(provider)
	return ok && scope == def.Scope
}

func credentialDir() (string, error) {
	root, err := portal.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "provider-credentials"), nil
}

func ensureCredentialDir() (string, error) {
	dir, err := credentialDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if err := secureCredentialDirectory(dir); err != nil {
		return "", err
	}
	return dir, nil
}

func credentialPath(provider string) (string, error) {
	if _, ok := definition(provider); !ok {
		return "", fmt.Errorf("지원하지 않는 provider adapter %q", provider)
	}
	dir, err := credentialDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, provider+".json"), nil
}

// Set validates and atomically stores one provider credential with user-only
// permissions (0600 on Unix; protected current-user/SYSTEM DACL on Windows).
func Set(provider string, credential Credential) error {
	def, ok := definition(provider)
	if !ok {
		return fmt.Errorf("지원하지 않는 provider adapter %q", provider)
	}
	if err := validateCredential(def, credential); err != nil {
		return err
	}
	if _, err := ensureCredentialDir(); err != nil {
		return err
	}
	path, err := credentialPath(provider)
	if err != nil {
		return err
	}
	return atomicWrite(path, func(w io.Writer) error {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(credential)
	})
}

func validateCredential(def providerDefinition, credential Credential) error {
	if credential.Key != strings.TrimSpace(credential.Key) || len(credential.Key) < 8 || len(credential.Key) > 4096 ||
		strings.ContainsAny(credential.Key, "\r\n\x00") {
		return fmt.Errorf("provider key는 앞뒤 공백·줄바꿈 없이 8~4096자여야 합니다")
	}
	if credential.Domain == "" {
		return nil
	}
	if !def.AllowDomain {
		return fmt.Errorf("%s credential은 domain metadata를 사용하지 않습니다", def.ID)
	}
	u, err := url.Parse(credential.Domain)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" ||
		u.User != nil || u.Fragment != "" {
		return fmt.Errorf("VWorld domain은 userinfo·fragment가 없는 HTTP(S) URL이어야 합니다")
	}
	return nil
}

// Get returns a key only when both the adapter ID and credential scope match the
// reviewed provider definition.
func Get(provider, scope string) (Credential, error) {
	def, ok := definition(provider)
	if !ok {
		return Credential{}, fmt.Errorf("지원하지 않는 provider adapter %q", provider)
	}
	if scope != def.Scope {
		return Credential{}, fmt.Errorf("provider credential scope 불일치: %s는 %s에만 사용할 수 있습니다", provider, def.Scope)
	}
	path, err := credentialPath(provider)
	if err != nil {
		return Credential{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Credential{}, fmt.Errorf("%w: `odeduck provider-key set %s` 로 저장하세요", ErrNotConfigured, provider)
	}
	if err != nil {
		return Credential{}, err
	}
	var credential Credential
	if err := json.Unmarshal(data, &credential); err != nil {
		return Credential{}, fmt.Errorf("%s provider credential 해석 실패: %w", provider, err)
	}
	if err := validateCredential(def, credential); err != nil {
		return Credential{}, fmt.Errorf("%s provider credential이 유효하지 않습니다: %w", provider, err)
	}
	return credential, nil
}

func Delete(provider string) error {
	path, err := credentialPath(provider)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func Status() ([]ProviderStatus, error) {
	statuses := make([]ProviderStatus, 0, len(providerDefinitions))
	for _, def := range providerDefinitions {
		status := ProviderStatus{Provider: def.ID, Scope: def.Scope}
		credential, err := Get(def.ID, def.Scope)
		switch {
		case err == nil:
			status.Configured = true
			status.Domain = credential.Domain
		case errors.Is(err, ErrNotConfigured):
		default:
			return nil, err
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

// ClearAll removes only the three fixed provider credential files. Public
// catalogues and browser/session state are outside this directory.
func ClearAll() error {
	roots, err := portal.ConfigDirsForCleanup()
	if err != nil {
		return err
	}
	var cleanupErrors []error
	for _, root := range roots {
		credentialRoot := filepath.Join(root, "provider-credentials")
		for _, def := range providerDefinitions {
			path := filepath.Join(credentialRoot, def.ID+".json")
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("%s (%s): %w", def.ID, root, err))
			}
		}
		entries, err := os.ReadDir(credentialRoot)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("provider temp (%s): %w", root, err))
			continue
		}
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), ".provider-key-") || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("provider temp %s (%s): %w", entry.Name(), root, err))
				continue
			}
			if !info.Mode().IsRegular() {
				continue
			}
			if err := os.Remove(filepath.Join(credentialRoot, entry.Name())); err != nil && !os.IsNotExist(err) {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("provider temp %s (%s): %w", entry.Name(), root, err))
			}
		}
	}
	return errors.Join(cleanupErrors...)
}

func atomicWrite(path string, write func(io.Writer) error) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".provider-key-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := secureCredentialFile(tmpName); err != nil {
		tmp.Close()
		return err
	}
	if err := write(tmp); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := replaceFile(tmpName, path); err != nil {
		return err
	}
	return syncParentDirectory(filepath.Dir(path))
}
