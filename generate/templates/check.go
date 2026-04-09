package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// runCheck performs a 3-tier verification that generate/templates.zip is up-to-date.
// It is invoked by hack/verify-codegen.sh via:
//
//	go run ./generate/templates check
func runCheck() error {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}

	zipPath := filepath.Join(repoRoot, "generate", "templates.zip")
	templatesDir := filepath.Join(repoRoot, "templates")

	var errs []error

	// Check 1 — Existence
	if _, err := os.Stat(zipPath); os.IsNotExist(err) {
		return fmt.Errorf("generate/templates.zip not found. Run 'make generate/templates.zip' first")
	}

	// Check 2 — Staleness (fast mtime check)
	zipInfo, err := os.Stat(zipPath)
	if err != nil {
		return fmt.Errorf("cannot stat generate/templates.zip: %w", err)
	}
	newerFound := ""
	_ = filepath.Walk(templatesDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if info.ModTime().After(zipInfo.ModTime()) {
			newerFound = path
			return filepath.SkipAll
		}
		return nil
	})
	if newerFound != "" {
		errs = append(errs, fmt.Errorf("generate/templates.zip is stale (file %s is newer). Run 'make generate/templates.zip'", newerFound))
	}

	// Check 3 — Content hash (definitive, catches branch-switch scenarios)
	existingData, err := os.ReadFile(zipPath)
	if err != nil {
		return fmt.Errorf("cannot read templates.zip: %w", err)
	}
	existingHash := sha256.Sum256(existingData)

	var buf bytes.Buffer
	if err := writeZip(&buf, templatesDir); err != nil {
		return fmt.Errorf("failed to regenerate zip for comparison: %w", err)
	}
	regeneratedHash := sha256.Sum256(buf.Bytes())

	if existingHash != regeneratedHash {
		errs = append(errs, fmt.Errorf("generate/templates.zip content mismatch. The regenerated zip differs from the existing one. Run 'make generate/templates.zip'"))
	}

	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "ERROR:", e)
		}
		return fmt.Errorf("generate/templates.zip is out of date")
	}

	fmt.Println("generate/templates.zip is up to date.")
	return nil
}

// findRepoRoot walks up from the current directory to find the repo root
// (identified by the presence of go.mod).
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find repo root (no go.mod found)")
		}
		dir = parent
	}
}
