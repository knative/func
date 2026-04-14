package cluster

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// downloadClient for binary downloads with generous timeout.
var downloadClient = &http.Client{Timeout: 5 * time.Minute}

// bins lists the binaries to download and manage. Checksums pins a hex
// SHA-256 per "<os>/<arch>" key; the installer refuses to run on a
// platform not present in the map.
var bins = []struct {
	Name      string
	Version   string
	URL       func(goos, goarch string) string
	Checksums map[string]string
}{
	{
		Name:      "kubectl",
		Version:   kubectlVersion,
		Checksums: kubectlChecksums,
		URL: func(goos, goarch string) string {
			return fmt.Sprintf("https://dl.k8s.io/v%s/bin/%s/%s/kubectl", kubectlVersion, goos, goarch)
		},
	},
	{
		Name:      "kind",
		Version:   kindVersion,
		Checksums: kindChecksums,
		URL: func(goos, goarch string) string {
			return fmt.Sprintf("https://github.com/kubernetes-sigs/kind/releases/download/v%s/kind-%s-%s", kindVersion, goos, goarch)
		},
	},
}

// ensureBins downloads required tool binaries if they are not already
// present at the correct version. Binaries are stored as <name>-<version>
// with a symlink <name> -> <name>-<version>. Strictly-older versions on
// disk are removed; unparseable or newer entries are left alone.
func ensureBins(ctx context.Context, cfg ClusterConfig, out io.Writer) error {
	goos, goarch := runtime.GOOS, runtime.GOARCH

	if goos != "linux" && goos != "darwin" {
		return fmt.Errorf("unsupported operating system %q: only linux and darwin are supported", goos)
	}

	if err := os.MkdirAll(cfg.BinDir(), 0o755); err != nil {
		return fmt.Errorf("creating bin directory: %w", err)
	}

	platform := goos + "/" + goarch
	for _, bin := range bins {
		sum, ok := bin.Checksums[platform]
		if !ok {
			return fmt.Errorf("no pinned checksum for %s on %s", bin.Name, platform)
		}
		if err := ensureBin(ctx, cfg.BinDir(), bin.Name, bin.Version, bin.URL(goos, goarch), sum, out); err != nil {
			return fmt.Errorf("installing %s: %w", bin.Name, err)
		}
	}

	fmt.Fprintln(out, green("DONE"))
	return nil
}

// ensureBin installs a single tool at the given version, verifying the
// downloaded bytes against the pinned SHA-256 wantSum.
func ensureBin(ctx context.Context, binDir, name, version, url, wantSum string, out io.Writer) error {
	fullName := fmt.Sprintf("%s-%s", name, version)
	path := filepath.Join(binDir, fullName)
	link := filepath.Join(binDir, name)

	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(out, "  %s %s (cached)\n", name, version)
		return updateLink(link, fullName)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("inspecting cache: %w", err)
	}

	fmt.Fprintf(out, "  %s %s (downloading)\n", name, version)
	if err := download(ctx, url, wantSum, path); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	removeOldVersions(binDir, name, version)
	return updateLink(link, fullName)
}

// download fetches url to dest atomically: it writes to dest+".tmp" while
// hashing, verifies against the pinned SHA-256 wantSum, then renames the
// tmp into place. A failure anywhere leaves dest untouched.
func download(ctx context.Context, url, wantSum, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := downloadClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}

	got := hex.EncodeToString(h.Sum(nil))
	if got != strings.ToLower(wantSum) {
		os.Remove(tmp)
		return fmt.Errorf("checksum mismatch: got %s", got)
	}

	return os.Rename(tmp, dest)
}

// updateLink atomically updates link to point to target. It creates a
// temporary symlink and renames it over link, so a failure leaves the
// previous link intact.
func updateLink(link, target string) error {
	tmp := link + ".tmp"
	_ = os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return fmt.Errorf("symlink: %w", err)
	}
	if err := os.Rename(tmp, link); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("symlink rename: %w", err)
	}
	return nil
}

// removeOldVersions removes "<name>-<version>" files whose version parses as
// semver and compares strictly less than current. Non-semver entries, the
// current version, and newer versions are left untouched.
func removeOldVersions(binDir, name, current string) {
	matches, _ := filepath.Glob(filepath.Join(binDir, name+"-*"))
	cur := "v" + current
	if !semver.IsValid(cur) {
		return
	}
	prefix := name + "-"
	for _, m := range matches {
		v := "v" + strings.TrimPrefix(filepath.Base(m), prefix)
		if !semver.IsValid(v) {
			continue
		}
		if semver.Compare(v, cur) < 0 {
			os.Remove(m)
		}
	}
}
