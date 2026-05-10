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
	fmt.Println("OK!")
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

	owner, repo, err := shared.RepoFromEnv()
	if err != nil {
		return err
	}
	ghClient := shared.NewGHClient(ctx)

	prTitle := fmt.Sprintf("chore: update Quarkus platform version to %s", latestVersion)
	exists, err := shared.PRExists(ctx, ghClient, owner, repo, func(title string) bool {
		return title == prTitle
	})
	if err != nil {
		return fmt.Errorf("cannot check for existing PR: %w", err)
	}
	if exists {
		fmt.Println("The PR already exists!")
		return nil
	}

	for _, pomPath := range []string{cePomPath, httpPomPath} {
		if err := updatePOM(pomPath, latestVersion); err != nil {
			return fmt.Errorf("cannot update %s: %w", pomPath, err)
		}
	}

	smokeCmd := []string{"make", "test-quarkus"}
	if err := shared.RunCmd(ctx, smokeCmd[0], smokeCmd[1:]...); err != nil {
		return fmt.Errorf("smoke test failed: %w", err)
	}

	branchName := fmt.Sprintf("update-quarkus-platform-%s", latestVersion)
	if err := shared.PrepareBranch(ctx, branchName, prTitle, []string{
		cePomPath, httpPomPath, "generate/zz_filesystem_generated.go",
	}); err != nil {
		return fmt.Errorf("cannot prepare branch: %w", err)
	}

	if err := shared.CreatePR(ctx, ghClient, owner, repo, prTitle, fmt.Sprintf("%s:%s", owner, branchName)); err != nil {
		return err
	}
	fmt.Println("The PR has been created!")
	return nil
}

func getLatestQuarkusVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, quarkusPlatformAPI, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
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
