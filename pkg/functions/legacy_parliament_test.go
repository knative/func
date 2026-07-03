package functions_test

// LEGACY PYTHON: parliament detection tests.

import (
	"os"
	"path/filepath"
	"testing"

	fn "knative.dev/func/pkg/functions"
)

// writeFile writes content to root/name, creating parent dirs, failing the test
// on error.
func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestIsLegacyParliament covers detection of old parliament-era python
// functions and the false-positive guards (a modern function and a WSGI
// `def main(environ, start_response)` must NOT be detected).
func TestIsLegacyParliament(t *testing.T) {
	tests := []struct {
		name    string
		runtime string
		files   map[string]string // relative path -> content
		want    bool
	}{
		{
			name:    "legacy http via Procfile",
			runtime: "python",
			files: map[string]string{
				"Procfile":         "web: python -m parliament .\n",
				"requirements.txt": "parliament-functions==0.1.0\n",
				"func.py":          "def main(context):\n    return \"OK\", 200\n",
			},
			want: true,
		},
		{
			name:    "legacy via 'from parliament import' (no Procfile)",
			runtime: "python",
			files: map[string]string{
				"func.py": "from parliament import Context\n\ndef main(context: Context):\n    return \"OK\", 200\n",
			},
			want: true,
		},
		{
			name:    "legacy cloudevents (Procfile + @event)",
			runtime: "python",
			files: map[string]string{
				"Procfile": "web: python -m parliament .\n",
				"func.py":  "from parliament import Context, event\n\n@event\ndef main(context):\n    return context.cloud_event.data\n",
			},
			want: true,
		},
		{
			name:    "modern function (new layout, no parliament)",
			runtime: "python",
			files: map[string]string{
				"pyproject.toml":       "[project]\nname = \"f\"\ndependencies = [\"func-python\"]\n",
				"function/__init__.py": "from .func import new\n",
				"function/func.py":     "class new:\n    async def handle(self, scope, receive, send):\n        ...\n",
			},
			want: false,
		},
		{
			name:    "false-positive guard: WSGI def main(environ, start_response)",
			runtime: "python",
			files: map[string]string{
				"func.py": "def main(environ, start_response):\n    start_response('200 OK', [])\n    return [b'hi']\n",
			},
			want: false,
		},
		{
			name:    "non-python runtime is never legacy",
			runtime: "go",
			files: map[string]string{
				"Procfile": "web: python -m parliament .\n",
			},
			want: false,
		},
		{
			name:    "Procfile with python3 and submodule import",
			runtime: "python",
			files: map[string]string{
				"Procfile": "web: python3 -m parliament .\n",
				"func.py":  "from parliament.invocation import Context\n\ndef main(context):\n    return \"OK\", 200\n",
			},
			want: true,
		},
		{
			name:    "import parliament as alias",
			runtime: "python",
			files: map[string]string{
				"func.py": "import parliament as p\n\ndef main(context):\n    return \"OK\", 200\n",
			},
			want: true,
		},
		{
			name:    "false-positive guard: import parliamentarian (different package)",
			runtime: "python",
			files: map[string]string{
				"func.py": "import parliamentarian\n\ndef main(context):\n    return parliamentarian.run()\n",
			},
			want: false,
		},
		{
			name:    "false-positive guard: commented-out parliament import",
			runtime: "python",
			files: map[string]string{
				"func.py": "# from parliament import Context  -- migrated away\nasync def handle(scope, receive, send):\n    ...\n",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for name, content := range tt.files {
				writeFile(t, root, name, content)
			}
			f := fn.Function{Root: root, Runtime: tt.runtime}
			if got := f.IsLegacyParliament(); got != tt.want {
				t.Errorf("IsLegacyParliament() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIsUnsupportedLegacyPython covers detection of the pre-v1.18 Procfile-based
// layouts other than parliament (old flask/wsgi templates): a root Procfile with
// no root pyproject.toml. Parliament is the supported exception (false), and the
// modern layout must never match.
func TestIsUnsupportedLegacyPython(t *testing.T) {
	tests := []struct {
		name    string
		runtime string
		files   map[string]string // relative path -> content
		want    bool
	}{
		{
			name:    "old flask template layout (gunicorn Procfile)",
			runtime: "python",
			files: map[string]string{
				"Procfile":         "web: gunicorn func:application --bind=0.0.0.0:8080 --access-logfile=-\n",
				"requirements.txt": "gunicorn==22.0.0\nFlask==2.2.5\n",
				"func.py":          "from flask import Flask\napplication = Flask(__name__)\n",
			},
			want: true,
		},
		{
			name:    "old wsgi template layout (gunicorn Procfile)",
			runtime: "python",
			files: map[string]string{
				"Procfile":         "web: gunicorn func:main --bind=0.0.0.0:8080 --access-logfile=-\n",
				"requirements.txt": "gunicorn==22.0.0\n",
				"func.py":          "def main(environ, start_response):\n    start_response('200 OK', [])\n    return [b'hi']\n",
			},
			want: true,
		},
		{
			name:    "parliament function is the supported exception",
			runtime: "python",
			files: map[string]string{
				"Procfile":         "web: python -m parliament .\n",
				"requirements.txt": "parliament-functions==0.1.0\n",
				"func.py":          "def main(context):\n    return \"OK\", 200\n",
			},
			want: false,
		},
		{
			name:    "modern function (pyproject.toml, no Procfile)",
			runtime: "python",
			files: map[string]string{
				"pyproject.toml":       "[project]\nname = \"f\"\n",
				"function/__init__.py": "from .func import new\n",
			},
			want: false,
		},
		{
			name:    "modern function with a stray Procfile (pyproject.toml wins)",
			runtime: "python",
			files: map[string]string{
				"pyproject.toml": "[project]\nname = \"f\"\n",
				"Procfile":       "web: gunicorn func:main\n",
			},
			want: false,
		},
		{
			name:    "no Procfile is not a legacy layout",
			runtime: "python",
			files: map[string]string{
				"func.py": "def main(environ, start_response):\n    ...\n",
			},
			want: false,
		},
		{
			name:    "non-python runtime never matches",
			runtime: "go",
			files: map[string]string{
				"Procfile": "web: gunicorn func:main\n",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for name, content := range tt.files {
				writeFile(t, root, name, content)
			}
			f := fn.Function{Root: root, Runtime: tt.runtime}
			if got := f.IsUnsupportedLegacyPython(); got != tt.want {
				t.Errorf("IsUnsupportedLegacyPython() = %v, want %v", got, tt.want)
			}
		})
	}
}
