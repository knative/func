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
	"golang.org/x/sync/semaphore"

	"knative.dev/func/pkg/creds"
	"knative.dev/func/pkg/oci"
)

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

func isECRRegistry(registry string) bool {
	if registry == "public.ecr.aws" {
		return true
	}
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

	errStr := err.Error()
	return strings.Contains(errStr, "credentials not found") ||
		strings.Contains(errStr, "no valid providers in chain") ||
		strings.Contains(errStr, "NoCredentialProviders") ||
		strings.Contains(errStr, "no AWS credentials")
}

type ecrCacheEntry struct {
	creds     oci.Credentials
	err       error
	createdAt time.Time
}

func GetECRCredentialLoader() []creds.CredentialsCallback {
	var (
		ecrHelper        *ecr.ECRHelper
		ecrKeychain      authn.Keychain
		ecrInitOnce      sync.Once
		ecrCache         sync.Map // registry (string) -> ecrCacheEntry
		ecrLookupSem     *semaphore.Weighted
		maxConcurrentECR = int64(2) // Limit to 2 concurrent lookups
	)

	initECR := func() {
		ecrHelper = ecr.NewECRHelper(ecr.WithLogger(io.Discard))
		ecrKeychain = authn.NewKeychainFromHelper(ecrHelper)
		ecrLookupSem = semaphore.NewWeighted(maxConcurrentECR)
	}

	return []creds.CredentialsCallback{
		func(registry string) (oci.Credentials, error) {
			if !isECRRegistry(registry) {
				return oci.Credentials{}, creds.ErrCredentialsNotFound
			}

			// Check cache first (TTL: 1 minute)
			if val, ok := ecrCache.Load(registry); ok {
				entry := val.(ecrCacheEntry)
				if time.Since(entry.createdAt) < 1*time.Minute {
					return entry.creds, entry.err
				}
				ecrCache.Delete(registry)
			}

			// Lazy initialize
			ecrInitOnce.Do(initECR)

			// Limit concurrent ECR lookups to prevent goroutine explosion
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := ecrLookupSem.Acquire(ctx, 1); err != nil {
				resErr := fmt.Errorf("ECR credential lookup timed out (queue full): %w", err)
				ecrCache.Store(registry, ecrCacheEntry{
					creds:     oci.Credentials{},
					err:       resErr,
					createdAt: time.Now(),
				})
				return oci.Credentials{}, resErr
			}
			defer ecrLookupSem.Release(1)

			// Perform the lookup WITHOUT spawning a goroutine
			// The semaphore limits concurrency, not a goroutine channel
			res, err := name.NewRegistry(registry)
			if err != nil {
				errMsg := fmt.Errorf("parse registry: %w", err)
				ecrCache.Store(registry, ecrCacheEntry{
					creds:     oci.Credentials{},
					err:       errMsg,
					createdAt: time.Now(),
				})
				return oci.Credentials{}, errMsg
			}

			authenticator, err := ecrKeychain.Resolve(res)
			if err != nil {
				if isAWSCredentialsNotFound(err) {
					ecrCache.Store(registry, ecrCacheEntry{
						creds:     oci.Credentials{},
						err:       creds.ErrCredentialsNotFound,
						createdAt: time.Now(),
					})
					return oci.Credentials{}, creds.ErrCredentialsNotFound
				}
				errMsg := fmt.Errorf("resolve ECR keychain: %w", err)
				ecrCache.Store(registry, ecrCacheEntry{
					creds:     oci.Credentials{},
					err:       errMsg,
					createdAt: time.Now(),
				})
				return oci.Credentials{}, errMsg
			}

			authCfg, err := authenticator.Authorization()
			if err != nil {
				if isAWSCredentialsNotFound(err) {
					ecrCache.Store(registry, ecrCacheEntry{
						creds:     oci.Credentials{},
						err:       creds.ErrCredentialsNotFound,
						createdAt: time.Now(),
					})
					return oci.Credentials{}, creds.ErrCredentialsNotFound
				}
				errMsg := fmt.Errorf("get authorization: %w", err)
				ecrCache.Store(registry, ecrCacheEntry{
					creds:     oci.Credentials{},
					err:       errMsg,
					createdAt: time.Now(),
				})
				return oci.Credentials{}, errMsg
			}

			if authCfg.Username == "" || authCfg.Password == "" {
				ecrCache.Store(registry, ecrCacheEntry{
					creds:     oci.Credentials{},
					err:       creds.ErrCredentialsNotFound,
					createdAt: time.Now(),
				})
				return oci.Credentials{}, creds.ErrCredentialsNotFound
			}

			creds := oci.Credentials{
				Username: authCfg.Username,
				Password: authCfg.Password,
			}

			// Don't cache success (tokens expire)
			return creds, nil
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
