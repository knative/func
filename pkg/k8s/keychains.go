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

func isAWSCredentialsNotFound(err error) bool {
	if err == nil {
		return false
	}
	// Check standard docker-credential-helpers sentinel
	if dockercreds.IsErrCredentialsNotFound(err) {
		return true
	}

	// Check for AWS SDK v1 errors if wrapped/accessible
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

	// Check for AWS SDK v2 (smithy) API errors
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

	// Last-resort fallback message checks
	errStr := err.Error()
	return strings.Contains(errStr, "credentials not found") ||
		strings.Contains(errStr, "no valid providers in chain") ||
		strings.Contains(errStr, "NoCredentialProviders") ||
		strings.Contains(errStr, "no AWS credentials")
}

type ecrCacheEntry struct {
	creds oci.Credentials
	err   error
}

func GetECRCredentialLoader() []creds.CredentialsCallback {
	var (
		ecrHelper   *ecr.ECRHelper
		ecrKeychain authn.Keychain
		ecrInitOnce sync.Once
		ecrCache    sync.Map // registry (string) -> ecrCacheEntry
	)
	initECR := func() {
		ecrHelper = ecr.NewECRHelper(ecr.WithLogger(io.Discard))
		ecrKeychain = authn.NewKeychainFromHelper(ecrHelper)
	}

	return []creds.CredentialsCallback{
		func(registry string) (oci.Credentials, error) {
			if !isECRRegistry(registry) {
				return oci.Credentials{}, creds.ErrCredentialsNotFound
			}

			// Check cache first (contains cached failures and timeouts)
			if val, ok := ecrCache.Load(registry); ok {
				entry := val.(ecrCacheEntry)
				return entry.creds, entry.err
			}

			res, err := name.NewRegistry(registry)
			if err != nil {
				return oci.Credentials{}, fmt.Errorf("parse registry: %w", err)
			}

			// Lazy initialize ECR helper and keychain
			ecrInitOnce.Do(initECR)

			// Add timeout to prevent hanging on AWS credential lookups
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			type result struct {
				creds oci.Credentials
				err   error
			}
			ch := make(chan result, 1)

			go func() {
				authenticator, err := ecrKeychain.Resolve(res)
				if err != nil {
					if isAWSCredentialsNotFound(err) {
						ch <- result{err: creds.ErrCredentialsNotFound}
					} else {
						ch <- result{err: fmt.Errorf("resolve ECR keychain: %w", err)}
					}
					return
				}

				authCfg, err := authenticator.Authorization()
				if err != nil {
					if isAWSCredentialsNotFound(err) {
						ch <- result{err: creds.ErrCredentialsNotFound}
					} else {
						ch <- result{err: fmt.Errorf("get authorization: %w", err)}
					}
					return
				}

				if authCfg.Username == "" || authCfg.Password == "" {
					ch <- result{err: creds.ErrCredentialsNotFound}
					return
				}

				ch <- result{
					creds: oci.Credentials{
						Username: authCfg.Username,
						Password: authCfg.Password,
					},
				}
			}()

			var resCreds oci.Credentials
			var resErr error

			select {
			case r := <-ch:
				if r.err != nil {
					resErr = r.err
				} else {
					resCreds = r.creds
				}
			case <-ctx.Done():
				resErr = fmt.Errorf("ECR credential lookup timed out: %w", ctx.Err())
			}

			// Cache only failures to avoid pinning expiring ECR auth tokens.
			if resErr != nil {
				ecrCache.Store(registry, ecrCacheEntry{creds: resCreds, err: resErr})
			}

			return resCreds, resErr
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
