package k8s

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/google"

	"knative.dev/func/pkg/creds"
	"knative.dev/func/pkg/oci"
)

func GetGoogleCredentialLoader() []creds.CredentialsCallback {
	return []creds.CredentialsCallback{
		func(registry string) (oci.Credentials, error) {
			if registry != "gcr.io" {
				return oci.Credentials{}, creds.ErrCredentialsNotFound // skip if not GCR
			}

			res, err := name.NewRegistry(registry)
			if err != nil {
				return oci.Credentials{}, fmt.Errorf("parse registry: %w", err)
			}

			authenticator, err := google.Keychain.Resolve(res)
			if err != nil {
				return oci.Credentials{}, fmt.Errorf("resolve google keychain: %w", err)
			}

			authCfg, err := authenticator.Authorization()
			if err != nil {
				return oci.Credentials{}, fmt.Errorf("get authorization: %w", err)
			}

			return oci.Credentials{
				Username: authCfg.Username,
				Password: authCfg.Password,
			}, nil
		},
	}
}

func GetECRCredentialLoader() []creds.CredentialsCallback {
	return []creds.CredentialsCallback{} // TODO: Implement ECR credentials loader
}

func GetACRCredentialLoader() []creds.CredentialsCallback {
	return []creds.CredentialsCallback{
		func(registry string) (oci.Credentials, error) {
			if !strings.HasSuffix(registry, ".azurecr.io") {
				return oci.Credentials{}, creds.ErrCredentialsNotFound
			}
			// TODO: Use Azure SDK to get access token instead of reading from file
			f, err := os.Open(path.Join(os.Getenv("HOME"), ".azure", "accessTokens.json"))
			if err != nil {
				return oci.Credentials{}, fmt.Errorf("open Azure access tokens: %w", err)
			}
			defer f.Close()

			var tokens []struct {
				AccessToken string `json:"accessToken"`
				Resource    string `json:"resource"`
			}

			if err := json.NewDecoder(f).Decode(&tokens); err != nil {
				return oci.Credentials{}, fmt.Errorf("decode Azure access tokens: %w", err)
			}

			target := "https://" + registry
			for _, t := range tokens {
				if t.Resource == target {
					return oci.Credentials{
						Username: "00000000-0000-0000-0000-000000000000",
						Password: t.AccessToken,
					}, nil
				}
			}
			return oci.Credentials{}, creds.ErrCredentialsNotFound
		},
	}
}
