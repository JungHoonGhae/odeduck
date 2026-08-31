package portal

import (
	"slices"
	"testing"
)

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
