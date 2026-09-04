package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/providerauth"
)

func TestProviderKeyCommandReadsSecretFromStdinAndNeverPrintsIt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	const secret = "SAFETY-SECRET-123"

	set := providerKeyCmd()
	set.SetArgs([]string{"set", "safetykorea"})
	set.SetIn(strings.NewReader(secret + "\n"))
	var setOut, setErr bytes.Buffer
	set.SetOut(&setOut)
	set.SetErr(&setErr)
	if err := set.Execute(); err != nil {
		t.Fatalf("set: %v", err)
	}
	if strings.Contains(setOut.String()+setErr.String(), secret) {
		t.Fatal("set output leaked provider credential")
	}
	stored, err := providerauth.Get("safetykorea", "https://www.safetykorea.kr/openapi/api/")
	if err != nil || stored.Key != secret {
		t.Fatalf("stored=%#v err=%v", stored, err)
	}

	oldFormat := flagFormat
	flagFormat = "json"
	t.Cleanup(func() { flagFormat = oldFormat })
	status := providerKeyCmd()
	status.SetArgs([]string{"status"})
	var statusOut bytes.Buffer
	status.SetOut(&statusOut)
	status.SetErr(&bytes.Buffer{})
	if err := status.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(statusOut.String(), secret) {
		t.Fatal("status leaked provider credential")
	}
	var statuses []providerauth.ProviderStatus
	if err := json.Unmarshal(statusOut.Bytes(), &statuses); err != nil {
		t.Fatalf("status JSON: %v\n%s", err, statusOut.String())
	}
	if len(statuses) != 3 || !statuses[0].Configured {
		t.Fatalf("statuses = %#v", statuses)
	}

	deleteCmd := providerKeyCmd()
	deleteCmd.SetArgs([]string{"delete", "safetykorea"})
	deleteCmd.SetOut(&bytes.Buffer{})
	deleteCmd.SetErr(&bytes.Buffer{})
	if err := deleteCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := providerauth.Get("safetykorea", "https://www.safetykorea.kr/openapi/api/"); err == nil {
		t.Fatal("delete left credential behind")
	}
}

func TestLogoutAlsoDeletesExternalProviderCredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	if err := providerauth.Set("safetykorea", providerauth.Credential{Key: "SAFETY-SECRET-123"}); err != nil {
		t.Fatal(err)
	}
	command := logoutCmd()
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := providerauth.Get("safetykorea", "https://www.safetykorea.kr/openapi/api/"); err == nil {
		t.Fatal("logout left external provider credential behind")
	}
}

func TestLogoutClearsProviderCredentialsWhenPortalCleanupFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	if err := providerauth.Set("safetykorea", providerauth.Credential{Key: "SAFETY-SECRET-123"}); err != nil {
		t.Fatal(err)
	}
	portalFailure := errors.New("portal cleanup failed")
	err := logoutAll(context.Background(), func(context.Context) error {
		return portalFailure
	}, providerauth.ClearAll)
	if !errors.Is(err, portalFailure) {
		t.Fatalf("logout error = %v, want portal cleanup failure", err)
	}
	if _, err := providerauth.Get("safetykorea", "https://www.safetykorea.kr/openapi/api/"); !errors.Is(err, providerauth.ErrNotConfigured) {
		t.Fatalf("provider credential remained after portal cleanup failure: %v", err)
	}
}

func TestCallCommandRoutesLinkPKThroughProviderCredentialWithoutLeakingIt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	const secret = "SAFETY-SECRET-123"
	if err := providerauth.Set("safetykorea", providerauth.Credential{Key: secret}); err != nil {
		t.Fatal(err)
	}

	const pk = "15116894"
	portalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/data/" + pk + "/openapi.do":
			_, _ = w.Write([]byte(`<ul><li><strong class="key">API 유형</strong><div class="value">LINK</div></li></ul>`))
		case "/tcs/dss/selectApiLinkUrl.do":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"linkUrl":"https://www.safetykorea.kr/release/openapi","status":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer portalServer.Close()

	oldBase, oldDelay := flagBaseURL, flagDelay
	flagBaseURL, flagDelay = portalServer.URL, 0
	t.Cleanup(func() { flagBaseURL, flagDelay = oldBase, oldDelay })
	command := callCmd()
	command.SetArgs([]string{"--pk", pk, "--op", "certificationDetail"})
	var stdout, stderr bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&stderr)
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "certNum") {
		t.Fatalf("call error = %v", err)
	}
	if strings.Contains(stdout.String()+stderr.String()+err.Error(), secret) {
		t.Fatal("call command leaked provider credential")
	}
}
