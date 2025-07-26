package oci

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/google"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/pkg/errors"
	progress "github.com/schollz/progressbar/v3"

	fn "knative.dev/func/pkg/functions"
)

// Pusher of OCI multi-arch layout directories.
type Pusher struct {
	Anonymous bool
	Insecure  bool
	Token     string
	Username  string
	Verbose   bool

	updates chan v1.Update
	done    chan bool
}

func NewPusher(insecure, anon, verbose bool) *Pusher {
	return &Pusher{
		Insecure:  insecure,
		Anonymous: anon,
		Verbose:   verbose,
		updates:   make(chan v1.Update, 10),
		done:      make(chan bool, 1),
	}
}

func (p *Pusher) Push(ctx context.Context, f fn.Function) (digest string, err error) {
	go p.handleUpdates(ctx)
	defer func() { p.done <- true }()
	buildDir, err := getLastBuildDir(f)
	if err != nil {
		return
	}

	var opts []name.Option
	if p.Insecure {
		opts = append(opts, name.Insecure)
	}
	// TODO: GitOps Tagging: tag :latest by default, :[branch] for pinned
	// environments and :[user]-[branch] for development/testing feature branches.
	// has been enabled, where branch is tag-encoded.
	ref, err := name.ParseReference(f.Build.Image, opts...)
	if err != nil {
		return
	}
	ii, err := layout.ImageIndexFromPath(filepath.Join(buildDir, "oci"))
	if err != nil {
		return
	}
	if err = p.writeIndex(ctx, ref, ii); err != nil {
		return
	}
	h, err := ii.Digest()
	if err != nil {
		return
	}
	digest = h.String()
	if p.Verbose {
		fmt.Printf("\ndigest: %s\n", h)
	}
	return
}

func (p *Pusher) handleUpdates(ctx context.Context) {
	var bar *progress.ProgressBar
	for {
		select {
		case update := <-p.updates:
			if bar == nil {
				bar = progress.NewOptions64(update.Total,
					progress.OptionSetVisibility(term.IsTerminal(int(os.Stdin.Fd()))),
					progress.OptionSetDescription("pushing"),
					progress.OptionShowCount(),
					progress.OptionShowBytes(true),
					progress.OptionShowElapsedTimeOnFinish())
			}
			_ = bar.Set64(update.Complete)
			continue
		case <-p.done:
			if bar != nil {
				_ = bar.Finish()
			}
			return
		case <-ctx.Done():
			if bar != nil {
				_ = bar.Finish()
			}
			return
		}
	}
}

// The last build directory is symlinked upon successful build.
func getLastBuildDir(f fn.Function) (string, error) {
	dir := filepath.Join(f.Root, fn.RunDataDir, "builds", "last")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return dir, fmt.Errorf("last build directory not found '%v'. Has it been built?", dir)
	}
	return dir, nil
}

// writeIndex to its defined registry.
func (p *Pusher) writeIndex(ctx context.Context, ref name.Reference, ii v1.ImageIndex) error {
	oo := []remote.Option{
		remote.WithContext(ctx),
		remote.WithProgress(p.updates),
	}

	if p.Insecure {
		t := remote.DefaultTransport.(*http.Transport).Clone()
		t.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
		oo = append(oo, remote.WithTransport(t))
	}

	if !p.Anonymous {
		a, err := p.authOption(ctx)
		if err != nil {
			return err
		}
		oo = append(oo, a)
	}

	return remote.WriteIndex(ref, ii, oo...)
}

// authOption selects an appropriate authentication option.
// If user provided = basic auth (secret is password)
// If only secret provided = bearer token auth
// If neither are provided = Returned is a cascading keychain auth mthod
// which performs the following in order:
// - Default Keychain (docker and podman config files)
// - Google Keychain
// - TODO: ECR Amazon
// - TODO: ACR Azure
func (p *Pusher) authOption(ctx context.Context) (remote.Option, error) {

	// Basic Auth if provided
	username, _ := ctx.Value(fn.PushUsernameKey{}).(string)
	password, _ := ctx.Value(fn.PushPasswordKey{}).(string)
	token, _ := ctx.Value(fn.PushTokenKey{}).(string)
	if username != "" && token != "" {
		return nil, errors.New("only one of username/password or token authentication allowed.  Received both a token and username")
	} else if token != "" {
		return remote.WithAuth(&authn.Bearer{Token: token}), nil
	} else if username != "" {
		return remote.WithAuth(&authn.Basic{Username: username, Password: password}), nil
	}

	// Default chain with resilient handling
	return remote.WithAuthFromKeychain(authn.NewMultiKeychain(
		localRegistryKeychain{},                      // Try localhost first
		resilientKeychain{inner: authn.DefaultKeychain}, // Podman and Docker config files with error handling
		google.Keychain,                              // Google
		// TODO: Integrate and test ECR and ACR credential helpers:
		// authn.NewKeychainFromHelper(ecr.ECRHelper{ClientFactory: api.DefaultClientFactory{}}),
		// authn.NewKeychainFromHelper(acr.ACRCredHelper{}),
	)), nil
}

// localRegistryKeychain returns anonymous auth for localhost registries
type localRegistryKeychain struct{}

func (k localRegistryKeychain) Resolve(resource authn.Resource) (authn.Authenticator, error) {
	registry := resource.RegistryStr()
	// Check if this is a localhost registry
	if registry == "localhost" || registry == "127.0.0.1" || 
	   registry == "localhost:5000" || registry == "localhost:50000" ||
	   registry == "127.0.0.1:5000" || registry == "127.0.0.1:50000" {
		return authn.Anonymous, nil
	}
	return authn.Anonymous, fmt.Errorf("not a localhost registry")
}

// resilientKeychain wraps another keychain and handles errors gracefully
type resilientKeychain struct {
	inner authn.Keychain
}

func (k resilientKeychain) Resolve(resource authn.Resource) (authn.Authenticator, error) {
	auth, err := k.inner.Resolve(resource)
	if err != nil {
		// If the error is about a missing credential helper, return anonymous auth
		// This handles the "docker-credential-desktop: executable file not found" case
		errStr := err.Error()
		if os.IsNotExist(err) || 
		   strings.Contains(errStr, "executable file not found") ||
		   strings.Contains(errStr, "not found in $PATH") ||
		   strings.Contains(errStr, "docker-credential-") {
			// Log warning to stderr
			fmt.Fprintf(os.Stderr, "Warning: credential helper not found, using anonymous auth: %v\n", err)
			return authn.Anonymous, nil
		}
		return nil, err
	}
	return auth, nil
}
