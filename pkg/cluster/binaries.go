package cluster

import (
	"archive/tar"
	"compress/gzip"
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
// platform not present in the map. ArchiveEntry, when non-empty, marks
// the URL as a .tar.gz archive containing the binary at that entry path
// (the checksum is verified against the archive, then the binary is
// extracted from it).
var bins = []struct {
	Name         string
	Version      string
	URL          func(goos, goarch string) string
	Checksums    map[string]string
	ArchiveEntry string
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
	{
		Name:         "act",
		Version:      actVersion,
		Checksums:    actChecksums,
		ArchiveEntry: "act",
		URL: func(goos, goarch string) string {
			// GitHub release asset names are case-insensitive, so goos
			// works directly. amd64 needs translation since act's release
			// uses x86_64 (a different name, not just different casing).
			arch := goarch
			if arch == "amd64" {
				arch = "x86_64"
			}
			return fmt.Sprintf("https://github.com/nektos/act/releases/download/v%s/act_%s_%s.tar.gz", actVersion, goos, arch)
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
		if err := ensureBin(ctx, cfg.BinDir(), bin.Name, bin.Version, bin.URL(goos, goarch), sum, bin.ArchiveEntry, out); err != nil {
			return fmt.Errorf("installing %s: %w", bin.Name, err)
		}
	}

	fmt.Fprintln(out, green("DONE"))
	return nil
}

// ensureBin installs a single tool at the given version, verifying the
// downloaded bytes against the pinned SHA-256 wantSum. If archiveEntry is
// non-empty, the URL is treated as a .tar.gz archive and the named entry
// is extracted from it after checksum verification.
//
// Cache hit is existence-only (os.Stat): we do not re-hash the on-disk file.
// For archived tools the pin is the *tarball* sum; after extract the path
// holds the binary, so a future "verify cached checksum" feature must not
// compare the extracted bytes to wantSum or act installs will false-fail.
func ensureBin(ctx context.Context, binDir, name, version, url, wantSum, archiveEntry string, out io.Writer) error {
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
	if archiveEntry != "" {
		if err := extractFromTarGz(path, archiveEntry); err != nil {
			return fmt.Errorf("extracting %s from archive: %w", archiveEntry, err)
		}
	}
	if err := os.Chmod(path, 0o755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	removeOldVersions(binDir, name, version)
	return updateLink(link, fullName)
}

// maxExtractedBinSize bounds the bytes extracted from a tarball entry.
// The archive is already SHA-256 verified, but a hard cap here defends
// in depth: it prevents a malicious tarball — were the pin ever wrong —
// from filling the disk or decompressing as a gzip-bomb. 256 MiB is
// generous for any tool binary we'd realistically install.
const maxExtractedBinSize = 256 * 1024 * 1024

// extractFromTarGz replaces the .tar.gz file at archivePath with the
// contents of the entry whose name matches entryName. The file is
// rewritten in place via an atomic rename, so a failure leaves the
// original archive untouched.
//
// Defensive choices: rejects non-regular file types (symlinks,
// hardlinks, devices, etc.); rejects entries declaring a size outside
// [0, maxExtractedBinSize]; uses io.CopyN to assert the body delivered
// matches the header's declared size (catches truncated streams).
func extractFromTarGz(archivePath, entryName string) error {
	// Extract into a sibling temp file first, then close every handle on the
	// original archive before renaming over it. On Windows a still-open
	// archivePath makes os.Rename return "Access is denied".
	tmp, err := extractTarGzEntryToTemp(archivePath, entryName)
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, archivePath); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// extractTarGzEntryToTemp opens archivePath, finds entryName, and writes it
// to archivePath+".extracted". Callers must close nothing; all handles are
// closed before return so Windows can rename over archivePath.
func extractTarGzEntryToTemp(archivePath, entryName string) (tmpPath string, err error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return "", fmt.Errorf("entry %q not found in archive", entryName)
		}
		if err != nil {
			return "", err
		}
		if hdr.Name != entryName {
			continue
		}
		if hdr.Typeflag != tar.TypeReg {
			return "", fmt.Errorf("entry %q has unexpected type 0x%x; only regular files are allowed", entryName, hdr.Typeflag)
		}
		if hdr.Size < 0 || hdr.Size > maxExtractedBinSize {
			return "", fmt.Errorf("entry %q declared size %d outside permitted range [0, %d]", entryName, hdr.Size, maxExtractedBinSize)
		}
		tmp := archivePath + ".extracted"
		w, err := os.Create(tmp)
		if err != nil {
			return "", err
		}
		if _, err := io.CopyN(w, tr, hdr.Size); err != nil {
			w.Close()
			os.Remove(tmp)
			return "", err
		}
		if err := w.Close(); err != nil {
			os.Remove(tmp)
			return "", err
		}
		// Explicit close before return so defer does not race with rename
		// after extractFromTarGz returns (defers run on function exit).
		_ = gz.Close()
		_ = f.Close()
		return tmp, nil
	}
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
