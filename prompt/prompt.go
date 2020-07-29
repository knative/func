package prompt

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Default delimiter between prompt and user input.  Includes a space such that
// overrides have full control of spacing.
const DefaultDelimiter = ": "

// DefaultRetryLimit imposed for a few reasons, not least of which infinite
// loops in tests.
const DefaultRetryLimit = 10

type prompt struct {
	in         io.Reader
	out        io.Writer
	label      string
	delim      string
	required   bool
	retryLimit int
}

type stringPrompt struct {
	prompt
	dflt string
}

type boolPrompt struct {
	prompt
	dflt bool
}

type Option func(*prompt)

func WithInput(in io.Reader) Option {
	return func(p *prompt) {
		p.in = in
	}
}

func WithOutput(out io.Writer) Option {
	return func(p *prompt) {
		p.out = out
	}
}

func WithDelimiter(d string) Option {
	return func(p *prompt) {
		p.delim = d
	}
}

func WithRequired(r bool) Option {
	return func(p *prompt) {
		p.required = r
	}
}

func WithRetryLimit(l int) Option {
	return func(p *prompt) {
		p.retryLimit = l
	}
}

func ForString(label string, dflt string, options ...Option) string {
	p := &stringPrompt{
		prompt: prompt{
			in:         os.Stdin,
			out:        os.Stdout,
			label:      label,
			delim:      DefaultDelimiter,
			retryLimit: DefaultRetryLimit,
		},
		dflt: dflt,
	}
	for _, o := range options {
		o(&p.prompt)
	}

	writeStringLabel(p)         // Write the label
	input, err := readString(p) // gather the input
	var attempt int
	for err != nil && attempt < p.retryLimit { // while there are errors
		attempt++
		writeError(err, &p.prompt) // write the error on its own line
		writeStringLabel(p)        // re-write the label
		input, err = readString(p) // re-read the input
	}
	return input
}

func writeStringLabel(p *stringPrompt) {
	_, err := p.out.Write([]byte(p.label))
	if err != nil {
		panic(err)
	}
	if p.dflt != "" {
		if p.label != "" {
			_, err = p.out.Write([]byte(" "))
			if err != nil {
				panic(err)
			}
		}
		fmt.Fprintf(p.out, "(%v)", p.dflt)
	}
	_, err = p.out.Write([]byte(p.delim))
	if err != nil {
		panic(err)
	}
}

func readString(p *stringPrompt) (s string, err error) {
	if s, err = bufio.NewReader(p.in).ReadString('\n'); err != nil {
		return
	}
	s = strings.TrimSpace(s)
	if s == "" {
		s = p.dflt
	}
	if s == "" && p.required {
		err = errors.New("please enter a value")
	}
	return
}

func ForBool(label string, dflt bool, options ...Option) bool {
	p := &boolPrompt{
		prompt: prompt{
			in:         os.Stdin,
			out:        os.Stdout,
			label:      label,
			delim:      DefaultDelimiter,
			retryLimit: DefaultRetryLimit,
		},
		dflt: dflt,
	}
	for _, o := range options {
		o(&p.prompt)
	}

	writeBoolLabel(p)         // write the prompt label
	input, err := readBool(p) // gather the input
	var attempt int
	for err != nil && attempt < p.retryLimit {
		attempt++
		writeError(err, &p.prompt) // write the error on its own line
		writeBoolLabel(p)          // re-write the label
		input, err = readBool(p)   // re-read the input
	}

	return input
}

func writeBoolLabel(p *boolPrompt) {
	_, err := p.out.Write([]byte(p.label))
	if err != nil {
		panic(err)
	}
	if p.label != "" {
		_, err = p.out.Write([]byte(" "))
		if err != nil {
			panic(err)
		}
	}
	if p.dflt {
		fmt.Fprint(p.out, "(Y/n)")
	} else {
		fmt.Fprint(p.out, "(y/N)")
	}
	_, err = p.out.Write([]byte(p.delim))
	if err != nil {
		panic(err)
	}
}

func readBool(p *boolPrompt) (bool, error) {
	reader := bufio.NewReader(p.in)
	var s string
	s, err := reader.ReadString('\n')
	if err != nil {
		return p.dflt, err
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return p.dflt, nil
	}
	if isTruthy(s) { // variants of 'true'
		return true, nil
	} else if isFalsy(s) {
		return false, nil
	}
	return strconv.ParseBool(s)
}

var truthy = regexp.MustCompile("(?i)y(?:es)?|1")
var falsy = regexp.MustCompile("(?i)n(?:o)?|0")

func isTruthy(confirm string) bool {
	return truthy.MatchString(confirm)
}

func isFalsy(confirm string) bool {
	return falsy.MatchString(confirm)
}

func writeError(err error, p *prompt) {
	_, _err := p.out.Write([]byte("\n"))
	if _err != nil {
		panic(_err)
	}
	fmt.Fprintln(p.out, err)
}
