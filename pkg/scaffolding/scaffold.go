package scaffolding

import (
	"fmt"
	"os"
	"path/filepath"

	"knative.dev/func/pkg/filesystem"
)

// Write scaffolding to a given path
//
// Scaffolding is a language-level operation which first detects the method
// signature used by the function's source code and then writes the
// appropriate scaffolding.
//
// NOTE: Scaffoding is not per-template, because a template is merely an
// example starting point for a Function implementation and should have no
// bearing on the shape that function can eventually take.  The language,
// and optionally invocation hint (For cloudevents) are used for this.  For
// example, there can be multiple templates which exemplify a given method
// signature, and the implementation can be switched at any time by the author.
// Language, by contrast, is fixed at time of initialization.
//
//	out:     the path to output scaffolding
//	src:      the path to the source code to scaffold
//	runtime: the expected runtime of the target source code "go", "node" etc.
//	invoke:  the optional invocatin hint (default "http")
//	fs:      filesytem which contains scaffolding at '[runtime]/scaffolding'
//	         (exclusive with 'repo')
func Write(out, src, runtime, invoke string, fs filesystem.Filesystem) (err error) {

	// detect the signature of the source code in the given location, presuming
	// a runtime and invocation hint (default "http")
	s, err := detectSignature(src, runtime, invoke)
	if err != nil {
		return err
	}

	// Path in the filesystem at which scaffolding is expected to exist
	d := fmt.Sprintf("%v/scaffolding/%v", runtime, s.String()) // fs uses / on all OSs
	if _, err := fs.Stat(d); err != nil {
		return ErrScaffoldingNotFound
	}

	// Copy from d -> out from the filesystem
	if err := filesystem.CopyFromFS(d, out, fs); err != nil {
		return ScaffoldingError{"filesystem copy failed", err}
	}

	// Copy the certs from the filesystem to the build directory
	if _, err := fs.Stat("certs"); err != nil {
		return ScaffoldingError{"certs directory not found in filesystem", err}
	}
	if err := filesystem.CopyFromFS("certs", out, fs); err != nil {
		return ScaffoldingError{"certs copy failed", err}
	}

	// Replace the 'f' link of the scaffolding (which is now incorrect) to
	// link to the function's root.
	rel, err := filepath.Rel(out, src)
	if err != nil {
		return ScaffoldingError{"error determining relative path to function source", err}
	}
	link := filepath.Join(out, "f")
	_ = os.Remove(link)
	if err = os.Symlink(rel, link); err != nil {
		return fmt.Errorf("error linking scaffolding to source %w", err)
	}
	return
}

// detectSignature returns the Signature of the source code at the given
// location assuming a provided runtime and invocation hint.
func detectSignature(src, runtime, invoke string) (s Signature, err error) {
	d, err := newDetector(runtime)
	if err != nil {
		return UnknownSignature, err
	}
	static, instanced, err := d.Detect(src)
	if err != nil {
		return
	}
	// Function must implement either a static handler or the instanced handler
	// but not both.
	if static && instanced {
		return s, fmt.Errorf("function may not implement both the static and instanced method signatures simultaneously")
	} else if !static && !instanced {
		return s, fmt.Errorf("function does not implement any known method signatures or does not compile")
	} else if instanced {
		return toSignature(true, invoke), nil
	} else {
		return toSignature(false, invoke), nil
	}
}
