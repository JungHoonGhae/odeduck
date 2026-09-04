package providerauth

import (
	"context"

	"github.com/JungHoonGhae/odeduck/internal/portal"
)

// Source adapts the two credential lifecycles to apicall.CredentialSource.
// data.go.kr keys are refreshed from the authenticated portal; external keys
// are explicit provider-scoped local credentials.
type Source struct{}

func (Source) DataGoKR(ctx context.Context) (string, error) { return portal.APIKey(ctx) }
func (Source) InvalidateDataGoKR()                          { portal.InvalidateCachedKey() }

func (Source) External(_ context.Context, provider, scope string) (string, string, error) {
	credential, err := Get(provider, scope)
	if err != nil {
		return "", "", err
	}
	return credential.Key, credential.Domain, nil
}
