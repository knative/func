package oci

import (
	"context"
	"fmt"
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

// Pusher of OCI multi-arch layout directories.
type Pusher struct {
	Insecure  bool
	Anonymous bool
	Verbose   bool
	Username  string
	Token     string

	updates chan v1.Update
	done    chan bool
}

func NewPusher(insecure, anon, verbose bool) *Pusher {
	fmt.Println(insecure, anon, verbose)
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

// The last build directory is symlinked upon successful build.
func getLastBuildDir(f fn.Function) (string, error) {
	dir := filepath.Join(f.Root, fn.RunDataDir, "builds", "last")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return dir, fmt.Errorf("last build directory not found '%v'. Has it been built?", dir)
	}
	return dir, nil
}

func (p *Pusher) writeIndex(ctx context.Context, ref name.Reference, ii v1.ImageIndex) error {
	// If we're set to anonymous, just try as-is and return on failure
	if p.Anonymous {
		return remote.WriteIndex(ref, ii,
			remote.WithContext(ctx),
			remote.WithProgress(p.updates))
	}

	// TODO: The below is untested and may only be useful for the test, since
	// a `docker login` utilizes the keychain for the httpasswd basic auth
	// credentials as well:
	if p.Username != "" && p.Token != "" {
		return remote.WriteIndex(ref, ii,
			remote.WithContext(ctx),
			remote.WithProgress(p.updates),
			remote.WithAuth(&authn.Basic{
				Username: p.Username,
				Password: p.Token,
			}))
	}

	// Otherwise use the keychain
	return remote.WriteIndex(ref, ii,
		remote.WithContext(ctx),
		remote.WithProgress(p.updates),
		remote.WithAuthFromKeychain(authn.DefaultKeychain))
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
