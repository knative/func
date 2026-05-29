package k8s

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	ecr "github.com/awslabs/amazon-ecr-credential-helper/ecr-login"
	"github.com/google/go-containerregistry/pkg/authn"
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

func isECRRegistry(registry string) bool {
	if registry == "public.ecr.aws" {
		return true
	}
	// Private ECR registries are always under a small set of known AWS partitions.
	isKnownECRDomain := strings.HasSuffix(registry, ".amazonaws.com") ||
		strings.HasSuffix(registry, ".amazonaws.com.cn") ||
		strings.HasSuffix(registry, ".sc2s.sgov.gov") ||
		strings.HasSuffix(registry, ".c2s.ic.gov")
	if !isKnownECRDomain {
		return false
	}

	return strings.Contains(registry, ".dkr.ecr.") || strings.Contains(registry, ".dkr.ecr-fips.")
}

func GetECRCredentialLoader() []creds.CredentialsCallback {
	return []creds.CredentialsCallback{
		func(registry string) (oci.Credentials, error) {
			if !isECRRegistry(registry) {
				return oci.Credentials{}, creds.ErrCredentialsNotFound
			}

			res, err := name.NewRegistry(registry)
			if err != nil {
				return oci.Credentials{}, fmt.Errorf("parse registry: %w", err)
			}

			ecrHelper := ecr.NewECRHelper(ecr.WithLogger(io.Discard))
			keychain := authn.NewKeychainFromHelper(ecrHelper)

			authenticator, err := keychain.Resolve(res)
			if err != nil {
				return oci.Credentials{}, fmt.Errorf("resolve ECR keychain: %w", err)
			}

			authCfg, err := authenticator.Authorization()
			if err != nil {
				return oci.Credentials{}, fmt.Errorf("get authorization: %w", err)
			}

			if authCfg.Username == "" || authCfg.Password == "" {
				return oci.Credentials{}, creds.ErrCredentialsNotFound
			}

			return oci.Credentials{
				Username: authCfg.Username,
				Password: authCfg.Password,
			}, nil
		},
	}
}

func GetACRCredentialLoader() []creds.CredentialsCallback {
	return []creds.CredentialsCallback{
		func(registry string) (oci.Credentials, error) {
			if !strings.HasSuffix(registry, ".azurecr.io") {
				return oci.Credentials{}, creds.ErrCredentialsNotFound
			}

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
