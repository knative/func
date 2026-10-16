package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"

	"knative.dev/func/hack/cmd/shared"
)

const (
	cePomPath   = "templates/quarkus/cloudevents/pom.xml"
	httpPomPath = "templates/quarkus/http/pom.xml"

	quarkusPlatformAPI = "https://code.quarkus.io/api/platforms"

	quarkusVersionTag  = "quarkus.platform.version"
	quarkusVersionExpr = `<quarkus\.platform\.version>([^<]+)</quarkus\.platform\.version>`
)

var quarkusVersionRe = regexp.MustCompile(quarkusVersionExpr)

type quarkusPlatformResponse struct {
	Platforms []struct {
		Streams []struct {
			Releases []struct {
				QuarkusCoreVersion string `json:"quarkusCoreVersion"`
			} `json:"releases"`
		} `json:"streams"`
	} `json:"platforms"`
}

func main() {
	ctx, stop := shared.NotifyContext(context.Background())
	defer stop()

	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	latestVersion, err := getLatestQuarkusVersion(ctx)
	if err != nil {
		return fmt.Errorf("cannot get latest Quarkus platform version: %w", err)
	}
	fmt.Printf("Latest Quarkus platform version: %s\n", latestVersion)

	ceVersion, err := versionFromPOM(cePomPath)
	if err != nil {
		return fmt.Errorf("cannot read version from %s: %w", cePomPath, err)
	}
	httpVersion, err := versionFromPOM(httpPomPath)
	if err != nil {
		return fmt.Errorf("cannot read version from %s: %w", httpPomPath, err)
	}

	if ceVersion == latestVersion && httpVersion == latestVersion {
		fmt.Println("Quarkus platform is up-to-date!")
		return nil
	}

	for _, pomPath := range []string{cePomPath, httpPomPath} {
		if err := updatePOM(pomPath, latestVersion); err != nil {
			return fmt.Errorf("cannot update %s: %w", pomPath, err)
		}
	}
	return nil
}

func getLatestQuarkusVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, quarkusPlatformAPI, nil)
	if err != nil {
		return "", err
	}
	resp, err := shared.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d from %s", resp.StatusCode, quarkusPlatformAPI)
	}

	var data quarkusPlatformResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}
	if len(data.Platforms) == 0 ||
		len(data.Platforms[0].Streams) == 0 ||
		len(data.Platforms[0].Streams[0].Releases) == 0 {
		return "", fmt.Errorf("unexpected response structure from Quarkus platform API")
	}
	v := data.Platforms[0].Streams[0].Releases[0].QuarkusCoreVersion
	if v == "" {
		return "", fmt.Errorf("quarkusCoreVersion is empty in API response")
	}
	return v, nil
}

func versionFromPOM(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	m := quarkusVersionRe.FindSubmatch(data)
	if len(m) < 2 {
		return "", fmt.Errorf("cannot find <%s> in %s", quarkusVersionTag, path)
	}
	return string(m[1]), nil
}

func updatePOM(path, newVersion string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	updated := quarkusVersionRe.ReplaceAll(data,
		[]byte(fmt.Sprintf("<%s>%s</%s>", quarkusVersionTag, newVersion, quarkusVersionTag)))
	return os.WriteFile(path, updated, 0644)
}
