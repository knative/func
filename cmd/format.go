package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

// write the output using the formatter's appropriate serialization function,
// returning any errors to the caller for graceful handling.
func write(out io.Writer, s Formatter, formatName string) error {
	switch Format(formatName) {
	case Human:
		return s.Human(out)
	case Plain:
		return s.Plain(out)
	case JSON:
		// Every JSON payload func emits is wrapped in the versioned envelope
		// (see cmd/json.go), whether it was requested with --json or with
		// --output json.  Wrapping here rather than at each call site keeps
		// the two spellings from drifting into separate dialects.
		var buf bytes.Buffer
		if err := s.JSON(&buf); err != nil {
			return err
		}
		return WriteJSONSuccess(out, json.RawMessage(buf.Bytes()))
	case YAML:
		return s.YAML(out)
	case URL:
		return s.URL(out)
	default:
		return fmt.Errorf("format not recognized: %v", formatName)
	}
}
