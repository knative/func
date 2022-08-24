package cmd

import (
	"fmt"
	"io"
)

type Format string

const (
	Human Format = "human" // Headers, indentation, justification etc.
	Plain        = "plain" // Suitable for cli automation via sed/awk etc.
	JSON         = "json"  // Technically a ⊆ yaml, but no one likes yaml.
	XML          = "xml"
	YAML         = "yaml"
	URL          = "url"
)

// formatter is any structure which has methods for serialization.
type Formatter interface {
	Human(io.Writer) error
	Plain(io.Writer) error
	JSON(io.Writer) error
	XML(io.Writer) error
	YAML(io.Writer) error
	URL(io.Writer) error
}

// write to the output the output of the formatter's appropriate serilization function.
// the command to exit with value 2.
func write(out io.Writer, s Formatter, formatName string) {
	var err error
	switch Format(formatName) {
	case Human:
		err = s.Human(out)
	case Plain:
		err = s.Plain(out)
	case JSON:
		err = s.JSON(out)
	case XML:
		err = s.XML(out)
	case YAML:
		err = s.YAML(out)
	case URL:
		err = s.URL(out)
	default:
		err = fmt.Errorf("format not recognized: %v\n", formatName)
	}
	if err != nil {
		panic(err)
	}
}
