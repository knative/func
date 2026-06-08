package k8s

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	ecr "github.com/awslabs/amazon-ecr-credential-helper/ecr-login"
	dockercreds "github.com/docker/docker-credential-helpers/credentials"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/google"

	"knative.dev/func/pkg/creds"
	"knative.dev/func/pkg/oci"
)

const (
	defaultECRTimeout  = 5 * time.Second
	defaultECRCacheTTL = 1 * time.Minute
)

type cachedCredential struct {
	creds  oci.Credentials
	err    error
	expiry time.Time
}

func GetGoogleCredentialLoader() []creds.CredentialsCallback {
	return []creds.CredentialsCallback{
		func(registry string) (oci.Credentials, error) {
			if registry != "gcr.io" {
				return oci.Credentials{}, creds.ErrCredentialsNotFound
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


func isAWSCredentialsNotFound(err error) bool {
	if err == nil {
		return false
	}
	if dockercreds.IsErrCredentialsNotFound(err) {
		return true
	}

	type awsError interface {
		Code() string
		Message() string
	}
	var awsErr awsError
	if errors.As(err, &awsErr) {
		if awsErr.Code() == "NoCredentialProviders" {
			return true
		}
	}

	type smithyAPIError interface {
		ErrorCode() string
		ErrorMessage() string
	}
	var smithyErr smithyAPIError
	if errors.As(err, &smithyErr) {
		if smithyErr.ErrorCode() == "NoCredentialProviders" {
			return true
		}
	}

	return false
}

func GetECRCredentialLoader() []creds.CredentialsCallback {
	var cache sync.Map

	return []creds.CredentialsCallback{
		func(registry string) (oci.Credentials, error) {

			if val, ok := cache.Load(registry); ok {
				cached := val.(cachedCredential)
				if time.Now().Before(cached.expiry) {
					return cached.creds, cached.err
				}
			}

			ctx, cancel := context.WithTimeout(context.Background(), defaultECRTimeout)
			defer cancel()

			ecrHelper := ecr.NewECRHelper(ecr.WithLogger(io.Discard), ecr.WithContext(ctx))
			ecrKeychain := authn.NewKeychainFromHelper(ecrHelper)

			res, err := name.NewRegistry(registry)
			if err != nil {
				return oci.Credentials{}, fmt.Errorf("parse registry: %w", err)
			}

			authenticator, err := ecrKeychain.Resolve(res)
			if err != nil {
				if isAWSCredentialsNotFound(err) {
					err = creds.ErrCredentialsNotFound
				} else {
					err = fmt.Errorf("resolve ECR keychain: %w", err)
				}
				cache.Store(registry, cachedCredential{
					err:    err,
					expiry: time.Now().Add(defaultECRCacheTTL),
				})
				return oci.Credentials{}, err
			}

			authCfg, err := authenticator.Authorization()
			if err != nil {
				if isAWSCredentialsNotFound(err) {
					err = creds.ErrCredentialsNotFound
				} else {
					err = fmt.Errorf("get authorization: %w", err)
				}
				cache.Store(registry, cachedCredential{
					err:    err,
					expiry: time.Now().Add(defaultECRCacheTTL),
				})
				return oci.Credentials{}, err
			}

			if authCfg.Username == "" || authCfg.Password == "" {
				err = creds.ErrCredentialsNotFound
				cache.Store(registry, cachedCredential{
					err:    err,
					expiry: time.Now().Add(defaultECRCacheTTL),
				})
				return oci.Credentials{}, err
			}

			credsVal := oci.Credentials{
				Username: authCfg.Username,
				Password: authCfg.Password,
			}
			cache.Store(registry, cachedCredential{
				creds:  credsVal,
				expiry: time.Now().Add(defaultECRCacheTTL),
			})
			return credsVal, nil
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
