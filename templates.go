package faas

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/markbates/pkger"
)

// Updating Templates:
// See documentation in ./templates/README.md

// DefautlTemplate is the default Function signature / environmental context
// of the resultant template.  All runtimes are expected to have at least
// an HTTP Handler ("http") and Cloud Events ("events")
const DefaultTemplate = "http"

// fileAccessor encapsulates methods for accessing template files.
type fileAccessor interface {
	Stat(name string) (os.FileInfo, error)
	Open(p string) (file, error)
	Walk(p string, wf filepath.WalkFunc) error
}

type file interface {
	io.ReadCloser
}

// When pkger is run, code analysis detects this Include statement,
// triggering the serializaation of the templates directory and all
// its contents into pkged.go, which is then made available via
// a pkger fileAccessor.
// Path is relative to the go module root.
func init() {
	_ = pkger.Include( "/templates")
}

type templateWriter struct {
	verbose   bool
	templates string
}

func (n templateWriter) Write(runtime, template, name, dest string) error {
	if template == "" {
		template = DefaultTemplate
	}

	// TODO: Confirm the dest path is empty?  This is currently in an earlier
	// step of the create process but future calls directly to initialize would
	// be better off being made safe.

	if isEmbedded(runtime, template) {
		return copyEmbedded(runtime, template, name, dest)
	}
	if n.templates != "" {
		return copyFilesystem(n.templates, runtime, template, dest)
	}
	return fmt.Errorf("A template for runtime '%v' template '%v' was not found internally and no custom template path was defined.", runtime, template)
}

func getEmbeddedTemplate(runtime, template, fnName string) fileAccessor {
	var result fileAccessor = embeddedAccessor{}

	src := filepath.Join(string(filepath.Separator), "templates", runtime, template)

	result = decorate(result, withChroot(src))

	if runtime == "quarkus" {

		tmplData := struct {
			GroupId    string
			ArtifactId string
		}{
			GroupId:    "dev.knative",
			ArtifactId: fnName,
		}

		result = decorate(result,
			withTemplating(tmplData, []string{
				"/src/main/java/functions/*.java",
				"/src/test/java/functions/*.java",
				"/*.md",
				"/pom.xml",
			}),
			withRenames(map[string]string{
				"/src/main/java/functions/": "/src/main/java/" + strings.ReplaceAll(tmplData.GroupId+"/"+tmplData.ArtifactId, ".", "/"),
				"/src/test/java/functions/": "/src/test/java/" + strings.ReplaceAll(tmplData.GroupId+"/"+tmplData.ArtifactId, ".", "/"),
			}),
		)
	}
	return result
}

func copyEmbedded(runtime, template, name, dest string) error {
	// Copy files to the destination
	// Example embedded path:
	//   /templates/go/http

	fa := getEmbeddedTemplate(runtime, template, name)
	err := copy(fa, dest, string(filepath.Separator))
	return err
}

func copyFilesystem(templatesPath, runtime, templateFullName, dest string) error {
	// ensure that the templateFullName is of the format "repoName/templateName"
	cc := strings.Split(templateFullName, "/")
	if len(cc) != 2 {
		return errors.New("Template name must be in the format 'REPO/NAME'")
	}
	repo := cc[0]
	template := cc[1]

	// Example FileSystem path:
	//   /home/alice/.config/faas/templates/boson-experimental/go/json
	src := filepath.Join(templatesPath, repo, runtime, template)
	fa := filesystemAccessor{}
	return copy(fa, dest, src)
}

func isEmbedded(runtime, template string) bool {
	_, err := pkger.Stat(filepath.Join("/templates", runtime, template))
	return err == nil
}

func copy(fs fileAccessor, dest, src string) error {
	return fs.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			err = os.MkdirAll(filepath.Join(dest, relPath), info.Mode())
			return err
		} else {
			srcF, err := fs.Open(path)
			if err != nil {
				return err
			}
			defer srcF.Close()
			destF, err := os.OpenFile(filepath.Join(dest, relPath), os.O_RDWR|os.O_CREATE|os.O_TRUNC, info.Mode())
			if err != nil {
				return err
			}
			defer destF.Close()
			_, err = io.Copy(destF, srcF)
			return err
		}
	})
}

type embeddedAccessor struct{}

func (a embeddedAccessor) Stat(path string) (os.FileInfo, error) {
	return pkger.Stat(path)
}

func (a embeddedAccessor) Open(path string) (file, error) {
	return pkger.Open(path)
}

func (a embeddedAccessor) Walk(dir string, wf filepath.WalkFunc) error {
	return pkger.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if strings.Contains(path, ":") {
			path = strings.Join(strings.Split(path, ":")[1:], "")
		}
		path = filepath.FromSlash(path)
		return wf(path, info, err)
	})
}

type filesystemAccessor struct{}

func (a filesystemAccessor) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

func (a filesystemAccessor) Open(path string) (file, error) {
	return os.Open(path)
}

func (a filesystemAccessor) Walk(p string, wf filepath.WalkFunc) error {
	return filepath.Walk(p, wf)
}

type chrootedFileAccessor struct {
	accessor fileAccessor
	root     string
}

func (c chrootedFileAccessor) Stat(name string) (os.FileInfo, error) {
	name = filepath.Join(string(filepath.Separator), name)
	name = filepath.Join(c.root, name)
	return c.accessor.Stat(name)
}

func (c chrootedFileAccessor) Open(name string) (file, error) {
	name = filepath.Join(string(filepath.Separator), name)
	name = filepath.Join(c.root, name)
	return c.accessor.Open(name)
}

func (c chrootedFileAccessor) Walk(name string, wf filepath.WalkFunc) error {
	name = filepath.Join(string(filepath.Separator), name)
	name = filepath.Join(c.root, name)
	return c.accessor.Walk(name, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(c.root, path)
		if err != nil {
			return err
		}
		path = filepath.Join(string(filepath.Separator), rel)
		return wf(path, info, err)
	})
}

type fileAccessorWithRenames struct {
	accessor fileAccessor
	renames  map[string]string
}

func cleanPath(path string) string {
	if filepath.IsAbs(path) {
		path = filepath.Clean(path)
	} else {
		path = filepath.Join(string(filepath.Separator), path)
	}
	return path
}

func rename(name string, renames map[string]string, rev bool) string {
	name = cleanPath(name)
	for key, value := range renames {
		key = cleanPath(key)
		value = cleanPath(value)

		if rev {
			key, value = value, key
		}

		if name == key || strings.HasPrefix(name, key+string(filepath.Separator)) {
			return filepath.Join(value, strings.TrimPrefix(name, key))
		}
	}
	return name
}

func (f fileAccessorWithRenames) Stat(name string) (os.FileInfo, error) {
	name = rename(name, f.renames, true)
	return f.accessor.Stat(name)
}

func (f fileAccessorWithRenames) Open(name string) (file, error) {
	name = rename(name, f.renames, true)
	return f.accessor.Open(name)
}

func (f fileAccessorWithRenames) Walk(name string, wf filepath.WalkFunc) error {
	name = rename(name, f.renames, true)
	return f.accessor.Walk(name, func(path string, info os.FileInfo, err error) error {
		return wf(rename(path, f.renames, false), info, err)
	})
}

type fileAccessorWithGoTextTemplateSubstitutes struct {
	accessor       fileAccessor
	templateParams interface{}
	patterns       []string
}

func (f fileAccessorWithGoTextTemplateSubstitutes) Stat(name string) (os.FileInfo, error) {
	return f.accessor.Stat(name)
}

func (f fileAccessorWithGoTextTemplateSubstitutes) Open(name string) (file, error) {
	var err error
	matched := false

	for _, pattern := range f.patterns {
		matched, err = filepath.Match(filepath.Clean(pattern), name)
		if err != nil {
			return nil, err
		}
		if matched {
			break
		}
	}

	if !matched {
		return f.accessor.Open(name)
	}

	fl, err := f.accessor.Open(name)
	if err != nil {
		return nil, err
	}
	defer fl.Close()

	data, err := ioutil.ReadAll(fl)
	if err != nil {
		return nil, err
	}

	tmpl, err := template.New("").Parse(string(data))
	if err != nil {
		return nil, err
	}

	pr, pw := io.Pipe()

	go func() {
		var err error
		defer func() {
			_ = pw.CloseWithError(err)
		}()
		err = tmpl.Execute(pw, f.templateParams)
	}()

	return pr, nil
}

func (f fileAccessorWithGoTextTemplateSubstitutes) Walk(name string, wf filepath.WalkFunc) error {
	return f.accessor.Walk(name, wf)
}

type fileAccessorDecorator func(fileAccessor) fileAccessor

func decorate(accessor fileAccessor, decorators ...fileAccessorDecorator) fileAccessor {
	for _, d := range decorators {
		accessor = d(accessor)
	}
	return accessor
}

func withChroot(root string) fileAccessorDecorator {
	return func(accessor fileAccessor) fileAccessor {
		return chrootedFileAccessor{
			accessor: accessor,
			root:     root,
		}
	}
}

func withRenames(renames map[string]string) fileAccessorDecorator {
	return func(accessor fileAccessor) fileAccessor {
		return fileAccessorWithRenames{
			accessor: accessor,
			renames:  renames,
		}
	}
}

func withTemplating(templateParams interface{}, patterns []string) fileAccessorDecorator {
	return func(accessor fileAccessor) fileAccessor {
		return fileAccessorWithGoTextTemplateSubstitutes{
			accessor:       accessor,
			templateParams: templateParams,
			patterns:       patterns,
		}
	}
}
