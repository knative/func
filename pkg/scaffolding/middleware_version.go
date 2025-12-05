package scaffolding

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"

	"github.com/BurntSushi/toml"
	"golang.org/x/mod/modfile"
	"knative.dev/func/pkg/filesystem"
)

// MiddlewareVersion returns the middleware version for the given function
//
//	src:     the path to the source code of the function
//	runtime: the expected runtime of the target source code "go", "node" etc.
//	invoke:  the optional invocation hint (default "http")
//	fs:      filesystem which contains scaffolding at '[runtime]/scaffolding'
//	         (exclusive with 'repo')
func MiddlewareVersion(src, runtime, invoke string, fs filesystem.Filesystem) (string, error) {
	s, err := detectSignature(src, runtime, invoke)
	if err != nil {
		return "", fmt.Errorf("failed to detect signature: %w", err)
	}

	vd, err := getVersionDetector(runtime)
	if err != nil {
		return "", fmt.Errorf("failed to get middleware version detector: %w", err)
	}

	return vd.Detect(fs, s)
}

type middlewareVersionDetector interface {
	Detect(fs filesystem.Filesystem, sig Signature) (string, error)
}

func getVersionDetector(runtime string) (middlewareVersionDetector, error) {
	switch runtime {
	case "go":
		return &golangMiddlewareVersionDetector{}, nil
	case "python":
		return &pythonMiddlewareVersionDetector{}, nil
	case "node":
		return &nodeMiddlewareVersionDetector{}, nil
	case "typescript":
		return &typescriptMiddlewareVersionDetector{}, nil
	case "quarkus":
		return &quarkusMiddlewareVersionDetector{}, nil
	case "java":
		return &springMiddlewareVersionDetector{}, nil
	case "rust":
		return &rustMiddlewareVersionDetector{}, nil
	default:
		return nil, fmt.Errorf("unsupported runtime: %s", runtime)
	}
}

type golangMiddlewareVersionDetector struct{}

func (d *golangMiddlewareVersionDetector) Detect(fs filesystem.Filesystem, sig Signature) (string, error) {
	gomodPath := fmt.Sprintf("go/scaffolding/%s/go.mod", sig.String())
	if _, err := fs.Stat(gomodPath); err != nil {
		return "", fmt.Errorf("failed to stat scaffolding go.mod: %w", err)
	}

	gomod, err := fs.Open(gomodPath)
	if err != nil {
		return "", fmt.Errorf("failed to open scaffolding go.mod: %w", err)
	}
	defer gomod.Close()

	content, err := io.ReadAll(gomod)
	if err != nil {
		return "", fmt.Errorf("failed to read scaffolding go.mod: %w", err)
	}

	f, err := modfile.Parse(gomodPath, content, nil)
	if err != nil {
		return "", fmt.Errorf("failed to parse scaffolding go.mod: %w", err)
	}

	for _, req := range f.Require {
		if req.Mod.Path == "knative.dev/func-go" {
			return req.Mod.Version, nil
		}
	}

	return "", fmt.Errorf("knative.dev/func-go dependency not found in %s", gomodPath)
}

type pythonMiddlewareVersionDetector struct{}

const funcPythonVersionRegex = `func-python[~^=><!]*([0-9]+\.[0-9]+\.[0-9]+)?`

func (d *pythonMiddlewareVersionDetector) Detect(fs filesystem.Filesystem, sig Signature) (string, error) {
	pyprojectPath := fmt.Sprintf("python/scaffolding/%s/pyproject.toml", sig.String())

	pyproject, err := fs.Open(pyprojectPath)
	if err != nil {
		return "", fmt.Errorf("failed to open scaffolding project.toml: %w", err)
	}
	defer pyproject.Close()

	content, err := io.ReadAll(pyproject)
	if err != nil {
		return "", fmt.Errorf("failed to read scaffolding project.toml: %w", err)
	}

	var config struct {
		Project struct {
			Dependencies []string `toml:"dependencies"`
		} `toml:"project"`
	}
	if err := toml.Unmarshal(content, &config); err != nil {
		return "", fmt.Errorf("failed to parse scaffolding project.toml: %w", err)
	}

	re := regexp.MustCompile(funcPythonVersionRegex)
	for _, dep := range config.Project.Dependencies {
		matches := re.FindStringSubmatch(dep)
		if len(matches) < 2 {
			continue
		}

		// If no version is specified, return empty string
		if len(matches[1]) == 0 {
			return "", nil
		}

		return matches[1], nil
	}

	return "", fmt.Errorf("func-python not found in %s", pyprojectPath)
}

type nodeMiddlewareVersionDetector struct{}

func (d *nodeMiddlewareVersionDetector) Detect(fs filesystem.Filesystem, sig Signature) (string, error) {
	invoke := "http"
	if sig == InstancedCloudevents || sig == StaticCloudevents {
		invoke = "cloudevents"
	}
	packageJsonPath := fmt.Sprintf("node/%s/package.json", invoke)

	packageJsonDetector := &packageJsonMiddlewareVersionDetector{}
	return packageJsonDetector.detect(fs, sig, packageJsonPath)
}

type typescriptMiddlewareVersionDetector struct{}

func (d *typescriptMiddlewareVersionDetector) Detect(fs filesystem.Filesystem, sig Signature) (string, error) {
	invoke := "http"
	if sig == InstancedCloudevents || sig == StaticCloudevents {
		invoke = "cloudevents"
	}
	packageJsonPath := fmt.Sprintf("typescript/%s/package.json", invoke)

	packageJsonDetector := &packageJsonMiddlewareVersionDetector{}
	return packageJsonDetector.detect(fs, sig, packageJsonPath)
}

type quarkusMiddlewareVersionDetector struct{}

func (d *quarkusMiddlewareVersionDetector) Detect(fs filesystem.Filesystem, sig Signature) (string, error) {
	invoke := "http"
	if sig == InstancedCloudevents || sig == StaticCloudevents {
		invoke = "cloudevents"
	}
	pomXmlPath := fmt.Sprintf("quarkus/%s/pom.xml", invoke)

	pomDetector := &pomMiddlewareVersionDetector{}
	return pomDetector.detect(fs, sig, pomXmlPath)
}

type springMiddlewareVersionDetector struct{}

func (d *springMiddlewareVersionDetector) Detect(fs filesystem.Filesystem, sig Signature) (string, error) {
	invoke := "http"
	if sig == InstancedCloudevents || sig == StaticCloudevents {
		invoke = "cloudevents"
	}
	pomXmlPath := fmt.Sprintf("springboot/%s/pom.xml", invoke)

	pomDetector := &pomMiddlewareVersionDetector{}
	return pomDetector.detect(fs, sig, pomXmlPath)
}

type rustMiddlewareVersionDetector struct{}

func (d *rustMiddlewareVersionDetector) Detect(_ filesystem.Filesystem, _ Signature) (string, error) {
	// we don't have any rust middleware, so simply return nothing
	return "", nil
}

type packageJsonMiddlewareVersionDetector struct{}

func (d *packageJsonMiddlewareVersionDetector) detect(fs filesystem.Filesystem, sig Signature, packageJsonPath string) (string, error) {
	packageJson, err := fs.Open(packageJsonPath)
	if err != nil {
		return "", fmt.Errorf("failed to open package.json: %w", err)
	}
	defer packageJson.Close()

	content, err := io.ReadAll(packageJson)
	if err != nil {
		return "", fmt.Errorf("failed to read package.json: %w", err)
	}

	var config struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(content, &config); err != nil {
		return "", fmt.Errorf("failed to parse project.json: %w", err)
	}

	semverRegex := regexp.MustCompile(`\d+\.\d+\.\d+`)
	for dep, version := range config.Dependencies {
		if dep == "faas-js-runtime" {
			return semverRegex.FindString(version), nil
		}
	}

	return "", fmt.Errorf("faas-js-runtime not found in %s", packageJsonPath)
}

type pomMiddlewareVersionDetector struct{}

func (d *pomMiddlewareVersionDetector) detect(fs filesystem.Filesystem, sig Signature, pomXmlPath string) (string, error) {
	pomXml, err := fs.Open(pomXmlPath)
	if err != nil {
		return "", fmt.Errorf("failed to open pom.xml: %w", err)
	}
	defer pomXml.Close()

	content, err := io.ReadAll(pomXml)
	if err != nil {
		return "", fmt.Errorf("failed to read pom.xml: %w", err)
	}

	re := regexp.MustCompile(`<quarkus\.platform\.version>(.*?)</quarkus\.platform\.version>`)
	match := re.FindSubmatch(content)
	if len(match) == 2 {
		return string(match[1]), nil
	}

	return "", fmt.Errorf("quarkus.platform.version property not found in %s", pomXmlPath)
}
