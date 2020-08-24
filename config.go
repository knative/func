package faas

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

// ConfigFileName is the name of the config's serialized form.
const ConfigFileName = ".faas.config"

// Config represents the serialized state of a Function's metadata.
// See the Function struct for attribute documentation.
type Config struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
	Runtime   string `yaml:"runtime"`
	Image     string `yaml:"image"`
	// Add new values to the toConfig/fromConfig functions.
}

// newConfig returns a Config populated from data serialized to disk if it is
// available.  Errors are returned if the path is not valid, if there are
// errors accessing an extant config file, or the contents of the file do not
// unmarshall.  A missing file at a valid path does not error but returns the
// empty value of Config.
func newConfig(root string) (c Config, err error) {
	filename := filepath.Join(root, ConfigFileName)
	if _, err = os.Stat(filename); os.IsNotExist(err) {
		err = nil // do not consider a missing config file an error
		return    // return the zero value of the config
	}
	bb, err := ioutil.ReadFile(filename)
	if err != nil {
		return
	}
	err = yaml.Unmarshal(bb, &c)
	return
}

// fromConfig returns a Function populated from config.
// Note that config does not include ancillary fields not serialized, such as Root.
func fromConfig(c Config) (f Function) {
	return Function{
		Name:      c.Name,
		Namespace: c.Namespace,
		Runtime:   c.Runtime,
		Image:     c.Image,
	}
}

// toConfig serializes a Function to a config object.
func toConfig(f Function) Config {
	return Config{
		Name:      f.Name,
		Namespace: f.Namespace,
		Runtime:   f.Runtime,
		Image:     f.Image,
	}
}

// writeConfig for the given Function out to disk at root.
func writeConfig(f Function) (err error) {
	path := filepath.Join(f.Root, ConfigFileName)
	c := toConfig(f)
	bb := []byte{}
	if bb, err = yaml.Marshal(&c); err != nil {
		return
	}
	return ioutil.WriteFile(path, bb, 0644)
}
