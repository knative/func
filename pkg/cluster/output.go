package cluster

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

// ANSI color codes matching the shell scripts' tput-based colors.
var (
	colorEnabled bool
	ansiRed      = "\033[1;31m"
	ansiGreen    = "\033[1;32m"
	ansiBlue     = "\033[1;34m"
	ansiYellow   = "\033[1;33m"
	ansiGrey     = "\033[1;90m"
	ansiReset    = "\033[0m"
)

func init() {
	// Honor the NO_COLOR convention (https://no-color.org): any non-empty
	// value disables ANSI output. Otherwise gate on stderr being a TTY —
	// all chatty/status output from this package goes there (stdout is
	// reserved for the machine-readable kubeconfig path), so the stderr
	// fd is what we actually need to check.
	colorEnabled = os.Getenv("NO_COLOR") == "" && term.IsTerminal(int(os.Stderr.Fd()))
}

func red(s string) string {
	if !colorEnabled {
		return s
	}
	return ansiRed + s + ansiReset
}

func green(s string) string {
	if !colorEnabled {
		return s
	}
	return ansiGreen + s + ansiReset
}

func blue(s string) string {
	if !colorEnabled {
		return s
	}
	return ansiBlue + s + ansiReset
}

func yellow(s string) string {
	if !colorEnabled {
		return s
	}
	return ansiYellow + s + ansiReset
}

func grey(s string) string {
	if !colorEnabled {
		return s
	}
	return ansiGrey + s + ansiReset
}

// prefixedWriter wraps an io.Writer, prepending a prefix to every line.
// It is safe for concurrent use from multiple goroutines.
type prefixedWriter struct {
	mu     sync.Mutex
	out    io.Writer
	prefix []byte
	buf    []byte
}

// newPrefixedWriter creates a writer that prepends prefix to every line written.
func newPrefixedWriter(out io.Writer, prefix string) *prefixedWriter {
	return &prefixedWriter{
		out:    out,
		prefix: []byte(prefix),
	}
}

func (pw *prefixedWriter) Write(p []byte) (n int, err error) {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	pw.buf = append(pw.buf, p...)

	for {
		idx := bytes.IndexByte(pw.buf, '\n')
		if idx < 0 {
			break
		}
		line := pw.buf[:idx+1]
		pw.buf = pw.buf[idx+1:]

		if _, err = pw.out.Write(pw.prefix); err != nil {
			return len(p), err
		}
		if _, err = pw.out.Write(line); err != nil {
			return len(p), err
		}
	}
	return len(p), nil
}

// Flush writes any remaining partial line.
func (pw *prefixedWriter) Flush() error {
	pw.mu.Lock()
	defer pw.mu.Unlock()
	if len(pw.buf) > 0 {
		if _, err := pw.out.Write(pw.prefix); err != nil {
			return err
		}
		if _, err := pw.out.Write(pw.buf); err != nil {
			return err
		}
		if _, err := pw.out.Write([]byte{'\n'}); err != nil {
			return err
		}
		pw.buf = nil
	}
	return nil
}

// status prints a blue status line (fixed message).
func status(out io.Writer, msg string) {
	fmt.Fprintln(out, blue(msg))
}

// statusf prints a blue status line with printf-style formatting.
func statusf(out io.Writer, format string, args ...any) {
	fmt.Fprintln(out, blue(fmt.Sprintf(format, args...)))
}

// warnf prints a yellow "Warning:" line with printf-style formatting.
func warnf(out io.Writer, format string, args ...any) {
	fmt.Fprintln(out, yellow("Warning: "+fmt.Sprintf(format, args...)))
}

// success prints a green checkmark status line with duration.
func success(out io.Writer, component string, d time.Duration) {
	fmt.Fprintf(out, "%s %s\n", green("✅ "+component), grey(formatDuration(d)))
}

// formatDuration returns a human-friendly duration string.
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("(%dms)", d.Milliseconds())
	}
	s := int(d.Seconds())
	if s < 60 {
		return fmt.Sprintf("(%ds)", s)
	}
	return fmt.Sprintf("(%dm%02ds)", s/60, s%60)
}
