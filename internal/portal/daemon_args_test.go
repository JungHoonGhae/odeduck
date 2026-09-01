package portal

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func isolatedUserConfigDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, ".config"))
	t.Setenv("APPDATA", filepath.Join(root, "AppData", "Roaming"))
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestConfigDirUsesOddsockForFreshInstall(t *testing.T) {
	configHome := isolatedUserConfigDir(t)

	got, err := configDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(configHome, configDirName)
	if got != want {
		t.Fatalf("configDir() = %q, want %q", got, want)
	}
}

func TestConfigDirReusesCompatibilityDirectories(t *testing.T) {
	for _, name := range compatibilityConfigDirNames {
		t.Run(name, func(t *testing.T) {
			configHome := isolatedUserConfigDir(t)
			legacy := filepath.Join(configHome, name)
			if err := os.MkdirAll(legacy, 0o700); err != nil {
				t.Fatal(err)
			}

			got, err := configDir()
			if err != nil {
				t.Fatal(err)
			}
			if got != legacy {
				t.Fatalf("configDir() = %q, want compatibility directory %q", got, legacy)
			}
		})
	}
}

func TestConfigDirPrefersOddsockWhenBothExist(t *testing.T) {
	configHome := isolatedUserConfigDir(t)
	current := filepath.Join(configHome, configDirName)
	legacy := filepath.Join(configHome, compatibilityConfigDirNames[0])
	for _, dir := range []string{legacy, current} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	got, err := configDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != current {
		t.Fatalf("configDir() = %q, want current directory %q", got, current)
	}
}

func TestConfigDirUsesPopulatedLegacyWhenCurrentIsEmpty(t *testing.T) {
	configHome := isolatedUserConfigDir(t)
	current := filepath.Join(configHome, configDirName)
	legacy := filepath.Join(configHome, compatibilityConfigDirNames[0])
	for _, dir := range []string{legacy, current} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(legacy, "catalog.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := configDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != legacy {
		t.Fatalf("configDir() = %q, want populated legacy directory %q", got, legacy)
	}
}

func TestConfigDirPrefersLegacyCredentialsOverCurrentPublicCatalog(t *testing.T) {
	configHome := isolatedUserConfigDir(t)
	current := filepath.Join(configHome, configDirName)
	legacy := filepath.Join(configHome, compatibilityConfigDirNames[0])
	for _, dir := range []string{legacy, current} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(current, "catalog.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "datagokr-session.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := configDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != legacy {
		t.Fatalf("configDir() = %q, want credential-bearing legacy directory %q", got, legacy)
	}
}

func TestConfigDirPrefersLegacySessionOverCurrentBrowserResidue(t *testing.T) {
	configHome := isolatedUserConfigDir(t)
	current := filepath.Join(configHome, configDirName)
	legacy := filepath.Join(configHome, compatibilityConfigDirNames[0])
	for _, dir := range []string{legacy, filepath.Join(current, "chrome-profile")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(current, "chrome-profile", "Local State"), []byte("preview"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "datagokr-session.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := configDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != legacy {
		t.Fatalf("configDir() = %q, want session-bearing legacy directory %q", got, legacy)
	}
}

func TestChromeArgsCapPortalTLSAt12(t *testing.T) {
	for name, args := range map[string][]string{
		"login":    loginBrowserArgs("/tmp/login-profile", "https://www.data.go.kr/sso/login.do"),
		"headless": headlessBrowserArgs("/tmp/headless-profile", 19434),
	} {
		if !slices.Contains(args, "--ssl-version-max=tls1.2") {
			t.Errorf("%s browser args do not cap TLS at 1.2: %v", name, args)
		}
	}
}
