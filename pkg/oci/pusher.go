package oci

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"golang.org/x/term"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	progress "github.com/schollz/progressbar/v3"

	fn "knative.dev/func/pkg/functions"
)

type Credentials struct {
	Username string
	Password string
	Token    string
}

func (c Credentials) Authorization() (*authn.AuthConfig, error) {
	return &authn.AuthConfig{
		Username:      c.Username,
		Password:      c.Password,
		IdentityToken: c.Token,
	}, nil
}

type CredentialsProvider func(ctx context.Context, image string) (Credentials, error)

type Opt func(*Pusher)

// Pusher of OCI multi-arch layout directories.
type Pusher struct {
	Anonymous           bool
	credentialsProvider CredentialsProvider

	Insecure bool
	Verbose  bool

	updates chan v1.Update
	done    chan bool

	transport http.RoundTripper
}

func EmptyCredentialsProvider(ctx context.Context, registry string) (Credentials, error) {
	return Credentials{}, nil
}

func WithCredentialsProvider(cp CredentialsProvider) Opt {
	return func(p *Pusher) {
		p.credentialsProvider = cp
	}
}

func WithVerbose(verbose bool) Opt {
	return func(pusher *Pusher) {
		pusher.Verbose = verbose
	}
}

func WithTransport(transport http.RoundTripper) Opt {
	return func(pusher *Pusher) {
		pusher.transport = transport
	}
}

func NewPusher(insecure, anon, verbose bool, opts ...Opt) *Pusher {
	result := &Pusher{
		credentialsProvider: EmptyCredentialsProvider,
		Insecure:            insecure,
		Anonymous:           anon,
		Verbose:             verbose,
		updates:             make(chan v1.Update, 10),
		done:                make(chan bool, 1),
		transport:           remote.DefaultTransport,
	}
	for _, opt := range opts {
		opt(result)
	}
	return result
}

func (p *Pusher) Push(ctx context.Context, f fn.Function) (digest string, err error) {
	credentials, _ := p.credentialsProvider(ctx, f.Build.Image)

	go p.handleUpdates(ctx)
	defer func() { p.done <- true }()
	buildDir, err := getBuildDir(f)
	if err != nil {
		return
	}

	// Extract registry from image to log which registry we're pushing to
	registry := "registry"
	if ref, err := name.ParseReference(f.Build.Image); err == nil {
		registry = ref.Context().RegistryStr()
	}

	// Log credentials being used (consistent with docker pusher)
	if credentials.Username != "" {
		fmt.Fprintf(os.Stderr, "Pushing function image to the registry %q using the %q user credentials\n", registry, credentials.Username)
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
	if err = p.writeIndex(ctx, ref, ii, credentials); err != nil {
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

// getBuildDir returns the build directory
func getBuildDir(f fn.Function) (string, error) {
	dir := filepath.Join(f.Root, fn.RunDataDir, fn.BuildDir)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return dir, fmt.Errorf("build directory not found '%v'. Has it been built?", dir)
	}
	return dir, nil
}

// writeIndex to its defined registry.
func (p *Pusher) writeIndex(ctx context.Context, ref name.Reference, ii v1.ImageIndex, creds Credentials) error {
	oo := []remote.Option{
		remote.WithContext(ctx),
		remote.WithProgress(p.updates),
		remote.WithTransport(p.transport),
	}

	if !p.Anonymous {
		oo = append(oo, remote.WithAuth(creds))
	}

	return remote.WriteIndex(ref, ii, oo...)
}
