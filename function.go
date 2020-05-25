package faas

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/net/publicsuffix"
)

type Function struct {
	root     string
	language string // will be empty unless initialized/until initialized
	name     string // will be empty unless initialized/until initialized.

	initializer Initializer
}

func NewFunction(root string) (f *Function, err error) {
	f = &Function{}

	// Default root to current directory, as an absolute path.
	if root == "" {
		root = "."
	}
	if root, err = filepath.Abs(root); err != nil {
		return
	}
	f.root = root

	// Populate with data from config if it exists.
	err = applyConfig(f, root)

	return
}

// DerivedName returns the name that will be used if path derivation is choosen, limited in its upward recursion.
// This is exposed for preemptive calculation for interactive confirmation, such as via a CLI.
func (f *Function) DerivedName(searchLimit int) string {
	return pathToDomain(f.root, searchLimit)
}

func (f *Function) Initialize(language, name string, domainSearchLimit int, initializer Initializer) (err error) {
	// Assert language is provided
	if language == "" {
		err = errors.New("language not specified")
		return
	}

	// If there exists contentious files (congig files for instance), this function may have already been initialized.
	files, err := contentiousFilesIn(f.root)
	if err != nil {
		return
	} else if len(files) > 0 {
		return errors.New(fmt.Sprintf("The chosen directory '%v' contains contentious files: %v.  Has the Service Function already been created?  Try either using a different directory, deleting the service function if it exists, or manually removing the files.", f.root, files))
	}

	// Ensure there are no non-hidden files, and again none of the aforementioned contentious files.
	empty, err := isEffectivelyEmpty(f.root)
	if err != nil {
		return
	} else if !empty {
		err = errors.New("The directory must be empty of visible files and recognized config files before it can be initialized.")
		return
	}

	// Derive a name if not provided
	if name == "" {
		name = pathToDomain(f.root, domainSearchLimit)
	}
	if name == "" {
		err = errors.New("Function name must be provided or be derivable from path")
		return
	}
	f.name = name

	// Write the template implementation in the appropriate language
	if err = initializer.Initialize(name, language, f.root); err != nil {
		return
	}
	// language was validated
	f.language = language

	// Write out the state as a config file and return.
	return writeConfig(f)
}

func (f *Function) Initialized() bool {
	// TODO: this should probably be more robust than checking what amounts to a
	// side-effect of the initialization process.
	return (f.language != "" && f.name != "")
}

// contentiousFiles are files which, if extant, preclude the creation of a
// service function rooted in the given directory.
var contentiousFiles = []string{
	".faas.yaml",
	".appsody-config.yaml",
}

// contentiousFilesIn the given directoy
func contentiousFilesIn(dir string) (contentious []string, err error) {
	files, err := ioutil.ReadDir(dir)
	for _, file := range files {
		for _, name := range contentiousFiles {
			if file.Name() == name {
				contentious = append(contentious, name)
			}
		}
	}
	return
}

// effectivelyEmpty directories are those which have no visible files,
// and none of the explicitly enumerated contentious files.
func isEffectivelyEmpty(dir string) (bool, error) {
	// Check for contentious files
	if contentious, err := contentiousFilesIn(dir); len(contentious) > 0 {
		return false, err
	}

	// Check for any non-hidden files
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, file := range files {
		if !strings.HasPrefix(file.Name(), ".") {
			return false, nil
		}
	}
	return true, nil
}

// Convert a path to a domain.
// Searches up the path string until a domain (TLD+1) is detected.
// Subdirectories are considered subdomains.
// Ex: Path:    "/home/users/jane/src/example.com/admin/www"
//     Returns: "www.admin.example.com"
// maxLevels is the number of directories to walk upwards beyond the current
// directory to determine domain (i.e. current directory is always considered.
// Zero indicates only consider last path element.)
func pathToDomain(path string, maxLevels int) string {
	var (
		// parts of the path, separated by os separator
		parts = strings.Split(path, string(os.PathSeparator))

		// subdomains derived from the path
		subdomains []string

		// domain derived from the path
		domain string
	)

	// Loop over parts from back to front (recursing upwards), building
	// optional subdomains until a root domain (TLD+1) is detected.
	for i := len(parts) - 1; i >= 0; i-- {
		part := parts[i]

		// Support limited recursion
		// Tests, for instance, need to be allowed to reliably fail by having their
		// recursion contained within ./testdata if recursion is set to -1, there
		// is no limit.  0 indicates only the current directory is considered.
		iteration := len(parts) - 1 - i
		if maxLevels >= 0 && iteration > maxLevels {
			break
		}

		// Detect TLD+1
		// If the current directory has a valid TLD plus one, it is a match.
		// This is determined by using the public suffices list, which includes
		// both ICANN managed TLDs as well as an extended list (matching, for
		// instance 'cluster.local')
		if suffix, _ := publicsuffix.EffectiveTLDPlusOne(part); suffix != "" {
			domain = part
			break // no directories above the nearest TLD+1 should be considered.
		}

		// Skip blanks
		// Path elements which are blank, such as in the case of a trailing slash
		// are ignored and the recursion continues, effectively collapsing ex: '//'.
		if part == "" {
			continue
		}

		// Build subdomain
		// Each path element which appears before the TLD+1 is a subdomain.
		// ex: '/home/users/jane/src/example.com/us-west-2/admin/www' creates the
		// subdomain []string{'www', 'admin', 'us-west-2'}
		subdomains = append(subdomains, part)
	}

	// Unable to derive domain
	// If the entire path was searched, but no parts matched a TLD+1, the domain
	// will be blank.  In this case, the path was insufficient to derive a domain
	// ex "/home/users/jane/src/test" contains no TLD, thus the final domain must
	// be explicitly provided.
	if domain == "" {
		return ""
	}

	// Prepend subdomains
	// If the path was a subdirectory within a TLD+1, these sudbomains
	// are prepended to the TLD+1 to create the final domain.
	// ex: '/home/users/jane/src/example.com/us-west-2/admin/www' yields
	// www.admin.use-west-2.example.com
	if len(subdomains) > 0 {
		subdomains = append(subdomains, domain)
		return strings.Join(subdomains, ".")
	}

	return domain
}
