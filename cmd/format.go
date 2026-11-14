package cmd

import (
	"fmt"
	"io"
	"strings"
)

type Format string

const (
	Human Format = "human" // Headers, indentation, justification etc.
	Plain        = "plain" // Suitable for cli automation via sed/awk etc.
	JSON         = "json"  // Technically a ⊆ yaml, but no one likes yaml.
	YAML         = "yaml"
	URL          = "url"
)

// formatter is any structure which has methods for serialization.
type Formatter interface {
	Human(io.Writer) error
	Plain(io.Writer) error
	JSON(io.Writer) error
	YAML(io.Writer) error
	URL(io.Writer) error
}

// validateOutputFormat returns an error if the format is not recognized.
func validateOutputFormat(formatName string) error {
	switch Format(strings.ToLower(formatName)) {
	case Human, Plain, JSON, YAML, URL:
		return nil
	default:
		return fmt.Errorf("format not recognized: %v", formatName)
	}
}

// write the output using the formatter's appropriate serialization function,
// returning any errors to the caller for graceful handling.
func write(out io.Writer, s Formatter, formatName string) error {
	switch Format(strings.ToLower(formatName)) {
	case Human:
		return s.Human(out)
	case Plain:
		return s.Plain(out)
	case JSON:
		return s.JSON(out)
	case YAML:
		return s.YAML(out)
	case URL:
		return s.URL(out)
	default:
		return fmt.Errorf("format not recognized: %v", formatName)
	}
}
