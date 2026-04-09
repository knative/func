package main

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	ignore "github.com/sabhiram/go-gitignore"
)

// runGenerate writes a binary zip of the templates/ directory to generate/templates.zip.
func runGenerate() error {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}
	zipPath := filepath.Join(repoRoot, "generate", "templates.zip")
	templatesDir := filepath.Join(repoRoot, "templates")

	f, err := os.OpenFile(zipPath, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	return writeZip(f, templatesDir)
}

// writeZip creates a zip archive of the given templatesDir into w,
// respecting .gitignore files found within.
func writeZip(w io.Writer, templatesDir string) error {
	zipWriter := zip.NewWriter(w)
	buff := make([]byte, 4*1024)

	// gitignoreCache caches compiled gitignore rules keyed by the directory containing .gitignore.
	gitignoreCache := map[string]*ignore.GitIgnore{}

	err := filepath.Walk(templatesDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		name, err := filepath.Rel(templatesDir, path)
		if err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		name = filepath.ToSlash(name)

		// Check if this path should be ignored by any .gitignore along the path.
		if shouldIgnore(path, name, info.IsDir(), templatesDir, gitignoreCache) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Load .gitignore from the current directory if it exists and not yet cached.
		if info.IsDir() {
			gitignorePath := filepath.Join(path, ".gitignore")
			if _, statErr := os.Stat(gitignorePath); statErr == nil {
				if _, cached := gitignoreCache[path]; !cached {
					compiled, compileErr := ignore.CompileIgnoreFile(gitignorePath)
					if compileErr == nil {
						gitignoreCache[path] = compiled
					}
				}
			}
		}

		if info.IsDir() {
			name = name + "/"
		}

		header := &zip.FileHeader{
			Name:   name,
			Method: zip.Deflate,
		}

		// Coercing permission to 755 for directories/executables and to 644 for non-executable files.
		// This is needed to ensure reproducible builds on machines with different values of `umask`.
		var mode fs.FileMode
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			mode = 0777 | fs.ModeSymlink
		case info.IsDir() || (info.Mode().Perm()&0111) != 0: // dir or executable
			mode = 0755
		case info.Mode()&fs.ModeType == 0: // regular file
			mode = 0644
		default:
			return fmt.Errorf("unsupported file type: %s", info.Mode().String())
		}
		header.SetMode(mode)

		zw, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			symlinkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			_, err = zw.Write([]byte(filepath.ToSlash(symlinkTarget)))
			return err
		case info.Mode()&fs.ModeType == 0: // regular file
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()

			_, err = io.CopyBuffer(zw, f, buff)
			return err
		default:
			return nil
		}
	})
	zipWriter.Close()
	return err
}

// shouldIgnore returns true if the given path matches any .gitignore rule from
// any parent directory up to (and including) the templates root.
// isDir controls whether a trailing slash is appended when matching, which is
// required for gitignore patterns like "target/" that only match directories.
func shouldIgnore(absPath, relFromTemplates string, isDir bool, templatesRoot string, cache map[string]*ignore.GitIgnore) bool {
	// Walk up from the file's parent directory to the templates root,
	// checking each directory's .gitignore rules.
	dir := filepath.Dir(absPath)
	for {
		if gi, ok := cache[dir]; ok {
			// Compute the path relative to the directory containing .gitignore.
			rel, err := filepath.Rel(dir, absPath)
			if err == nil {
				rel = filepath.ToSlash(rel)
				// Append trailing slash for directories so that patterns like
				// "target/" (directory-only) are matched correctly.
				if isDir {
					rel = rel + "/"
				}
				if gi.MatchesPath(rel) {
					return true
				}
			}
		}

		// Stop after processing the templates root directory.
		if dir == templatesRoot || !strings.HasPrefix(dir, templatesRoot) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return false
}
