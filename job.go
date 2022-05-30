package function

import (
	"fmt"
	"os"
	"path/filepath"
)

// Job represents a running Function job (presumably started by this process'
// Runner instance.
type Job struct {
	Function Function
	Port     string
	Errors   chan error
	onStop   func()
}

// Create a new Job which represents a running Function task by providing
// the port on which it was started, a channel on which runtime errors can
// be received, and a stop function.
func NewJob(f Function, port string, errs chan error, onStop func()) (*Job, error) {
	j := &Job{
		Function: f,
		Port:     port,
		Errors:   errs,
		onStop:   onStop,
	}
	return j, j.save() // Everything is a file:  save instance data to disk.
}

// Stop the Job, running the provided stop delegate and removing runtime
// metadata from disk.
func (j *Job) Stop() {
	_ = j.remove() // Remove representation on disk
	j.onStop()
}

func (j *Job) save() error {
	instancesDir := filepath.Join(j.Function.Root, RunDataDir, "instances")
	// job metadata is stored in <root>/.func/instances
	mkdir(instancesDir)

	// create a file <root>/.func/instances/<port>
	file, err := os.Create(filepath.Join(instancesDir, j.Port))
	if err != nil {
		return err
	}
	return file.Close()

	// Store the effective port for use by other client instances, possibly
	// in other processes, such as to run Invoke from other terminal in CLI apps.
	/*
		if err := writeFunc(f, "port", []byte(port)); err != nil {
			return j, err
		}
		return j, nil
	*/
}

func (j *Job) remove() error {
	filename := filepath.Join(j.Function.Root, RunDataDir, "instances", j.Port)
	return os.Remove(filename)
}

// jobPorts returns all the ports on which an instance of the given Function is
// running.  len is 0 when not running.
// Improperly initialized or nonexistent (zero value) Functions are considered
// to not be running.
func jobPorts(f Function) []string {
	if f.Root == "" || !f.Initialized() {
		return []string{}
	}
	instancesDir := filepath.Join(f.Root, RunDataDir, "instances")
	mkdir(instancesDir)

	files, err := os.ReadDir(instancesDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading %v", instancesDir)
		return []string{}
	}
	ports := []string{}
	for _, f := range files {
		ports = append(ports, f.Name())
	}
	return ports
}
