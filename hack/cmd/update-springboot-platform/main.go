package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"

	"github.com/blang/semver/v4"
	"gopkg.in/yaml.v3"

	"knative.dev/func/hack/cmd/shared"
)

const (
	cePomPath   = "templates/springboot/cloudevents/pom.xml"
	httpPomPath = "templates/springboot/http/pom.xml"

	springBootReleasesAPI = "https://api.github.com/repos/spring-projects/spring-boot/releases/latest"
	springCloudBOMURL     = "https://raw.githubusercontent.com/spring-io/start.spring.io/main/start-site/src/main/resources/application.yml"

	springCloudVersionTag  = "spring-cloud.version"
	springCloudVersionExpr = `<spring-cloud\.version>([^<]+)</spring-cloud\.version>`
)

// parentVersionRe matches the version inside the spring-boot-starter-parent block.
var (
	parentVersionRe      = regexp.MustCompile(`(<artifactId>spring-boot-starter-parent</artifactId>\s*<version>)([^<]+)(</version>)`)
	springCloudVersionRe = regexp.MustCompile(springCloudVersionExpr)
)

type springBootRelease struct {
	TagName string `json:"tag_name"`
	Draft   bool   `json:"draft"`
}

// springCloudMapping is one entry from the start.spring.io BOM.
type springCloudMapping struct {
	CompatibilityRange string `yaml:"compatibilityRange"`
	Version            string `yaml:"version"`
}

// springCloudBOM is the minimal structure we need from start.spring.io's application.yml.
type springCloudBOM struct {
	Initializr struct {
		Env struct {
			Boms map[string]struct {
				Mappings []springCloudMapping `yaml:"mappings"`
			} `yaml:"boms"`
		} `yaml:"env"`
	} `yaml:"initializr"`
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
	latestVersion, err := getLatestSpringBootVersion(ctx)
	if err != nil {
		return fmt.Errorf("cannot get latest Spring Boot version: %w", err)
	}
	if latestVersion == "" {
		fmt.Println("Spring Boot platform latest version is not ready to use!")
		return nil
	}
	fmt.Printf("Latest Spring Boot version: %s\n", latestVersion)

	ceVersion, err := parentVersionFromPOM(cePomPath)
	if err != nil {
		return fmt.Errorf("cannot read version from %s: %w", cePomPath, err)
	}
	httpVersion, err := parentVersionFromPOM(httpPomPath)
	if err != nil {
		return fmt.Errorf("cannot read version from %s: %w", httpPomPath, err)
	}

	if ceVersion == latestVersion && httpVersion == latestVersion {
		fmt.Println("Spring Boot platform is up-to-date!")
		return nil
	}

	springCloudVersion, err := getCompatibleSpringCloudVersion(ctx, latestVersion)
	if err != nil {
		return fmt.Errorf("cannot find compatible spring-cloud version: %w", err)
	}
	fmt.Printf("Compatible spring-cloud version: %s\n", springCloudVersion)

	for _, pomPath := range []string{cePomPath, httpPomPath} {
		if err := updatePOM(pomPath, latestVersion, springCloudVersion); err != nil {
			return fmt.Errorf("cannot update %s: %w", pomPath, err)
		}
	}
	return nil
}

func getLatestSpringBootVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, springBootReleasesAPI, nil)
	if err != nil {
		return "", err
	}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := shared.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d from %s", resp.StatusCode, springBootReleasesAPI)
	}

	var release springBootRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	if release.Draft {
		return "", nil
	}
	return strings.TrimPrefix(release.TagName, "v"), nil
}

func parentVersionFromPOM(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	m := parentVersionRe.FindSubmatch(data)
	if len(m) < 4 {
		return "", fmt.Errorf("cannot find spring-boot-starter-parent version in %s", path)
	}
	return string(m[2]), nil
}

func updatePOM(path, newVersion, newSpringCloudVersion string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	updated := parentVersionRe.ReplaceAll(data, []byte("${1}"+newVersion+"${3}"))
	updated = springCloudVersionRe.ReplaceAll(updated,
		[]byte(fmt.Sprintf("<%s>%s</%s>", springCloudVersionTag, newSpringCloudVersion, springCloudVersionTag)))
	return os.WriteFile(path, updated, 0644)
}

func getCompatibleSpringCloudVersion(ctx context.Context, springBootVersion string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, springCloudBOMURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := shared.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d from %s", resp.StatusCode, springCloudBOMURL)
	}

	var bom springCloudBOM
	if err := yaml.NewDecoder(resp.Body).Decode(&bom); err != nil {
		return "", fmt.Errorf("cannot decode spring-cloud BOM YAML: %w", err)
	}

	sc, ok := bom.Initializr.Env.Boms["spring-cloud"]
	if !ok {
		return "", fmt.Errorf("spring-cloud entry not found in BOM")
	}

	return resolveSpringCloudVersion(springBootVersion, sc.Mappings)
}

// resolveSpringCloudVersion finds the spring-cloud version compatible with the
// given springBootVersion by evaluating each mapping's compatibilityRange.
//
// Range format mirrors Maven version ranges:
//   - "[begin,end)" — begin inclusive, end exclusive
//   - "begin"       — begin inclusive, no upper bound
func resolveSpringCloudVersion(springBootVersion string, mappings []springCloudMapping) (string, error) {
	target, err := semver.ParseTolerant(springBootVersion)
	if err != nil {
		return "", fmt.Errorf("cannot parse Spring Boot version %q: %w", springBootVersion, err)
	}

	for _, m := range mappings {
		r := m.CompatibilityRange

		if strings.HasPrefix(r, "[") {
			// "[begin,end)" — strip the surrounding brackets by index, matching
			// the JS original's slice(1,-1), so that the closing ")" is removed
			// correctly (strings.Trim would only remove "[" and "]", leaving ")").
			if len(r) < 2 {
				continue
			}
			inner := r[1 : len(r)-1]
			parts := strings.SplitN(inner, ",", 2)
			if len(parts) != 2 {
				continue
			}
			begin, err := semver.ParseTolerant(strings.TrimSpace(parts[0]))
			if err != nil {
				continue
			}
			end, err := semver.ParseTolerant(strings.TrimSpace(parts[1]))
			if err != nil {
				continue
			}
			if target.GTE(begin) && target.LT(end) {
				return m.Version, nil
			}
		} else {
			// open-ended lower bound
			begin, err := semver.ParseTolerant(r)
			if err != nil {
				continue
			}
			if target.GTE(begin) {
				return m.Version, nil
			}
		}
	}

	return "", fmt.Errorf("no compatible spring-cloud version found for Spring Boot %s", springBootVersion)
}
