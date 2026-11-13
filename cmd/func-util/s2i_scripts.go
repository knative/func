package main

import (
	"fmt"
	"net/url"
	"path/filepath"
)

// resolveScriptsURL resolves a relative file:// scripts URL against the
// function's root. The CLI gives the scaffolded scripts relative to the
// function (file://.func/build/bin), while s2i would resolve them against the
// working directory, which is the root of the source and holds the function
// only when the function is not in a subdirectory of it. Other URLs are
// returned as they are.
func resolveScriptsURL(raw, funcRoot string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("cannot parse image script url %q: %w", raw, err)
	}
	if u.Scheme != "file" {
		return raw, nil
	}
	// s2i reads a file URL as Host joined with Path, since a relative path
	// parses with its first element as Host.
	p := filepath.Join(u.Host, u.Path)
	if filepath.IsAbs(p) {
		return raw, nil
	}
	return (&url.URL{Scheme: "file", Path: filepath.Join(funcRoot, p)}).String(), nil
}
