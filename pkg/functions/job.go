package functions

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
)

const runsDir = "runs"

// Job represents a running function job (presumably started by this process'
// Runner instance.
// In order for this to function along with the noop runner used by client,
// the zero value of the struct is set up to noop without errors.
type Job struct {
	Function Function
	Host     string
	Port     string
	Errors   chan error
	onStop   func() error
	verbose  bool
}

// Create a new Job which represents a running function task by providing
// the port on which it was started, a channel on which runtime errors can
// be received, and a stop function.
func NewJob(f Function, host, port string, errs chan error, onStop func() error, verbose bool) (j *Job, err error) {
	j = &Job{
		Function: f,
		Host:     host,
		Port:     port,
		Errors:   errs,
		onStop:   onStop,
		verbose:  verbose,
	}
	if !f.Initialized() {
		return j, errors.New("initialized function required to create job")
	}
	if j.Port == "" {
		return j, errors.New("port required to create job")
	}
	if j.Errors == nil {
		j.Errors = make(chan error, 1)
	}
	if j.onStop == nil {
		j.onStop = func() error { return nil }
	}
	if err = cleanupJobDirs(j); err != nil {
		return
	}
	if j.verbose {
		fmt.Printf("mkdir -p %v\n", j.Dir())
	}
	return j, os.MkdirAll(j.Dir(), os.ModePerm)
}

// Stop the Job, running the provided stop delegate and removing runtime
// metadata from disk.
func (j *Job) Stop() error {
	if j.verbose {
		fmt.Printf("rm %v\n", j.Dir())
	}
	if err := os.RemoveAll(j.Dir()); err != nil {
		fmt.Fprintf(os.Stderr, "warning: unable to remove run directory. %v", err)
	}
	return j.onStop()
}

// Directory within which all data about this current job is placed.
// ${f.Root}/.func/runs/${j.Port}
func (j *Job) Dir() string {
	return filepath.Join(funcJobsDir(j.Function), j.Port)
}

// Directory within which all runs (jobs) are held for the given function.
// ${f.Root}/.func/runs/
func funcJobsDir(f Function) string {
	return filepath.Join(f.Root, RunDataDir, runsDir)
}

// cleanupJobDirs removes any orphaned jobs' disk representation
func cleanupJobDirs(j *Job) error {
	dd, _ := os.ReadDir(funcJobsDir(j.Function))
	for _, d := range dd {
		if !d.IsDir() {
			continue // ignore files in the directory (like a readme)
		}
		if _, err := strconv.Atoi(d.Name()); err != nil {
			continue // ignore directories that aren't integers (ports)
		}
		ln, err := net.Listen("tcp", ":"+d.Name())
		if err != nil {
			continue // ignore if we can't bind to the port (running or invalid port)
		}
		_ = ln.Close()
		orphanedJobDir := filepath.Join(funcJobsDir(j.Function), d.Name())
		if j.verbose {
			fmt.Printf("No process listening on port %v.  Removing its job directory\n", d.Name())
			fmt.Printf("rm %v\n", orphanedJobDir)
		}
		return os.RemoveAll(orphanedJobDir)
	}
	return nil
}

// jobPorts returns all the ports on which an instance of the given function is
// running.  len is 0 when not running.
// Improperly initialized or nonexistent (zero value) functions are considered
// to not be running.
func jobPorts(f Function) []string {
	if f.Root == "" || !f.Initialized() {
		return []string{}
	}
	jobsDir := funcJobsDir(f)
	if _, err := os.Stat(jobsDir); err != nil {
		return []string{} // never started, so path does not exist
	}

	files, err := os.ReadDir(jobsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading %v", jobsDir)
		return []string{}
	}
	ports := []string{}
	for _, f := range files {
		ports = append(ports, f.Name())
	}
	// TODO: validate it's a directory whose name parses as an integer?
	return ports
}
