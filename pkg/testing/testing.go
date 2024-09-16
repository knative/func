// package testing includes minor testing helpers.
//
// These helpers include extensions to the testing nomenclature which exist to
// ease the development of tests for functions.  It is mostly just syntactic
// sugar and closures for creating an removing test directories etc.
// It was originally included in each of the requisite testing packages, but
// since we use both private-access enabled tests (in the function package),
// as well as closed-box tests (in function_test package), and they are gradually
// increasing in size and complexity, the choice was made to choose a small
// dependency over a small amount of copying.
//
// Another reason for including these in a separate locaiton is that they will
// have no tags such that no combination of tags can cause them to either be
// missing or interfere with eachother (a problem encountered with knative
// tooling which by default runs tests with all tags enabled simultaneously)
package testing

import (
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/cgi"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Using the given path, create it as a new directory and return a deferrable
// which will remove it.
// usage:
//
//	defer using(t, "testdata/example.com/someExampleTest")()
func Using(t *testing.T, root string) func() {
	t.Helper()
	mkdir(t, root)
	return func() {
		rm(t, root)
	}
}

// mkdir creates a directory as a test helper, failing the test on error.
func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
}

// rm a directory as a test helper, failing the test on error.
func rm(t *testing.T, dir string) {
	t.Helper()
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
}

// FileExists checks whether file on the specified path exists
func FileExists(t *testing.T, filePath string) (bool, error) {
	_, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Within the given root creates the directory, CDs to it, and rturns a
// closure that when executed (intended in a defer) removes the given dirctory
// and returns the caller to the initial working directory.
// usage:
//
//	defer within(t, "somedir")()
func Within(t *testing.T, root string) func() {
	t.Helper()
	cwd := pwd(t)
	mkdir(t, root)
	cd(t, root)
	return func() {
		cd(t, cwd)
		rm(t, root)
	}
}

// Mktemp creates a temporary directory, CDs the current processes (test) to
// said directory, and returns the path to said directory.
// Usage:
//
//	path, rm := Mktemp(t)
//	defer rm()
//	CWD is now 'path'
//
// errors encountererd fail the current test.
func Mktemp(t *testing.T) (string, func()) {
	t.Helper()
	tmp := t.TempDir()
	owd := pwd(t)
	cd(t, tmp)
	return tmp, func() {
		cd(t, owd)
	}
}

// Fromtemp is like Mktemp, but does not bother returing the temp path.
func Fromtemp(t *testing.T) func() {
	_, done := Mktemp(t)
	return done
}

// pwd prints the current working directory.
// errors fail the test.
func pwd(t *testing.T) string {
	t.Helper()
	d, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// cd changes directory to the given directory.
// errors fail the given test.
func cd(t *testing.T, dir string) {
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
}

// ServeRepositry [name] from ./testdata/[name] returning URL at which
// the named repository is available.
// Must be called before any helpers which change test working directory
// such as fromTempDirectory(t)
func ServeRepo(name string, t *testing.T) string {
	t.Helper()

	gitRoot := t.TempDir()

	// copy repo to the temp dir
	err := filepath.Walk(filepath.Join("./testdata", name), func(path string, fi fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relp, err := filepath.Rel("./testdata", path)
		if err != nil {
			return fmt.Errorf("cannot get relpath: %v", err)
		}

		dest := filepath.Join(gitRoot, relp)

		switch {
		case fi.Mode().IsRegular():
			var in, out *os.File
			in, err = os.Open(path)
			if err != nil {
				return fmt.Errorf("cannot open source file: %v", err)
			}
			defer in.Close()
			out, err = os.OpenFile(dest, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return fmt.Errorf("cannot open destination file: %v", err)
			}
			defer out.Close()
			_, err = io.Copy(out, in)
			if err != nil {
				return fmt.Errorf("cannot copy data: %v", err)
			}
		case fi.IsDir():
			err = os.MkdirAll(dest, 0755)
			if err != nil {
				return fmt.Errorf("cannot mkdir: %v", err)
			}
		default:
			return fmt.Errorf("unsupported file type")
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	url := RunGitServer(gitRoot, t)

	return fmt.Sprintf("%v/%v", url, name)
}

// WithExecutable creates an executable of the given name and source in a temp
// directory which is then added to PATH.  Returned is a deferrable which will
// clean up both the script and PATH.
func WithExecutable(t *testing.T, name, goSrc string) {
	var err error
	binDir := t.TempDir()

	newPath := binDir + string(os.PathListSeparator) + os.Getenv("PATH")
	t.Setenv("PATH", newPath)

	goSrcPath := filepath.Join(binDir, fmt.Sprintf("%s.go", name))

	err = os.WriteFile(goSrcPath,
		[]byte(goSrc),
		0400)
	if err != nil {
		t.Fatal(err)
	}

	if runtime.GOOS == "windows" {
		name = name + ".exe"
	}

	binaryPath := filepath.Join(binDir, name)

	cmd := exec.Command("go", "build", "-o="+binaryPath, goSrcPath)
	o, err := cmd.CombinedOutput()
	if err != nil {
		t.Log(string(o))
		t.Fatal(err)
	}
}

// RunGitServer starts serving git HTTP server and returns its address
func RunGitServer(root string, t *testing.T) (url string) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	url = l.Addr().String()

	cmd := exec.Command("git", "--exec-path")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	server := &http.Server{
		Handler: &cgi.Handler{
			Path: filepath.Join(strings.Trim(string(out), "\n"), "git-http-backend"),
			Env:  []string{"GIT_HTTP_EXPORT_ALL=true", fmt.Sprintf("GIT_PROJECT_ROOT=%s", root)},
		},
	}

	go func() {
		err = server.Serve(l)
		if err != nil && !strings.Contains(err.Error(), "Server closed") {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		}
	}()

	t.Cleanup(func() {
		server.Close()
	})

	return "http://" + url
}

// FromTempDirectory moves the test into a new temporary directory and
// clears all known interfering environment variables.  Returned is the
// path to the somewhat isolated test environment.
// Note that KUBECONFIG is also set to testdata/default_kubeconfig which can
// be used for tests which are explicitly checking logic which depends on
// kube context.
func FromTempDirectory(t *testing.T) string {
	t.Helper()
	ClearEnvs(t)

	// We have to define KUBECONFIG, or the file at ~/.kube/config (if extant)
	// will be used (disrupting tests by using the current user's environment).
	// The test kubeconfig set below has the current namespace set to 'func'
	// NOTE: the below settings affect unit tests only, and we do explicitly
	// want all unit tests to start in an empty environment with tests "opting in"
	// to config, not opting out.
	t.Setenv("KUBECONFIG", filepath.Join(Cwd(), "testdata", "default_kubeconfig"))

	// By default unit tests presum no config exists unless provided in testdata.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	t.Setenv("KUBERNETES_SERVICE_HOST", "")

	// creates and CDs to a temp directory
	d, done := Mktemp(t)

	// Done and Reset
	// NOTE:
	// NO CLI command should require resetting viper.  If a CLI test
	// is failing, and the following fixes the problem, it's probably because
	// an instance of a command is being reused multiple times in the same
	// test when a new instance of the command struct should instead be
	// created for each test case:
	// t.Cleanup(func() { done(); viper.Reset() })
	t.Cleanup(done)

	return d
}

// Cwd returns the current working directory or panic if unable to determine.
func Cwd() (cwd string) {
	cwd, err := os.Getwd()
	if err != nil {
		panic(fmt.Sprintf("Unable to determine current working directory: %v", err))
	}
	return cwd
}

// ClearEnvs sets all environment variables with the prefix of FUNC_ to
// empty (unsets) for the duration of the test t and is used when
// a test needs to completely clear func-releated envs prior to running.
func ClearEnvs(t *testing.T) {
	t.Helper()
	for _, v := range os.Environ() {
		if strings.HasPrefix(v, "FUNC_") {
			parts := strings.SplitN(v, "=", 2)
			t.Setenv(parts[0], "")
		}
	}
}
