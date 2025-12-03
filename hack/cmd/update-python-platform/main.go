package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"syscall"
)

const (
	funcPythonPackage      = "func-python"
	funcPythonVersionRegex = `"func-python[~^=><!]*([0-9]+\.[0-9]+\.[0-9]+)?"`
)

var pyprojectPaths = []string{
	"templates/python/scaffolding/instanced-http/pyproject.toml",
	"templates/python/scaffolding/instanced-cloudevents/pyproject.toml",
}

type PyPIRelease struct {
	Info struct {
		Version string `json:"version"`
	} `json:"info"`
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		cancel()
		<-sigs
		os.Exit(130)
	}()

	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	// Get latest version from PyPI
	latestVersion, err := getLatestPyPIVersion(ctx, funcPythonPackage)
	if err != nil {
		return fmt.Errorf("cannot get latest version from PyPI: %w", err)
	}

	fmt.Printf("Latest %s version on PyPI: %s\n", funcPythonPackage, latestVersion)

	// Check current versions in pyproject.toml files
	allUpToDate := true
	for _, path := range pyprojectPaths {
		currentVersion, err := getVersionFromPyproject(path)
		if err != nil {
			return fmt.Errorf("cannot get version from %s: %w", path, err)
		}

		fmt.Printf("Current version in %s: %q\n", path, currentVersion)

		if currentVersion != latestVersion {
			allUpToDate = false
		}
	}

	if allUpToDate {
		fmt.Println("func-python is up-to-date!")
		return nil
	}

	// Update files
	if err := updateFiles(latestVersion); err != nil {
		return err
	}

	fmt.Println("Files updated successfully!")
	return nil
}

func getLatestPyPIVersion(ctx context.Context, packageName string) (string, error) {
	url := fmt.Sprintf("https://pypi.org/pypi/%s/json", packageName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("PyPI returned status %d", resp.StatusCode)
	}

	var release PyPIRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}

	return release.Info.Version, nil
}

func getVersionFromPyproject(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	re := regexp.MustCompile(funcPythonVersionRegex)
	matches := re.FindSubmatch(data)
	if len(matches) < 2 {
		return "", fmt.Errorf("cannot find func-python dependency in %s", path)
	}

	// If no version is specified, return empty string
	if len(matches[1]) == 0 {
		return "", nil
	}

	return string(matches[1]), nil
}

func updateFiles(newVersion string) error {
	for _, path := range pyprojectPaths {
		if err := updatePyproject(path, newVersion); err != nil {
			return fmt.Errorf("cannot update %s: %w", path, err)
		}
		fmt.Printf("Updated %s to version %s\n", path, newVersion)
	}
	return nil
}

func updatePyproject(path, newVersion string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// Replace func-python version
	re := regexp.MustCompile(funcPythonVersionRegex)
	newData := re.ReplaceAll(data, []byte(fmt.Sprintf(`"func-python==%s"`, newVersion)))

	return os.WriteFile(path, newData, 0644)
}
