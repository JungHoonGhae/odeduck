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

func TestConfigDirUsesOdeduckForFreshInstall(t *testing.T) {
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
