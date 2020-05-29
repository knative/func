package progress

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Bar - a simple, unobtrusive progress indicator
// Usage:
//   bar := New()
//   bar.SetTotal(3)
//   defer bar.Done()
//   bar.Increment("Step 1")
//   bar.Increment("Step 2")
//   bar.Increment("Step 3")
//   bar.Complete("Done")
//
// Instantiation creates a progress bar consisiting of an optional spinner
// prefix followed by an indicator of the current step, the total steps, and a
// trailing message The behavior differs substantially if verbosity is enbled
// or not. When in verbose mode, it is expected that the process is otherwise
// printing status information to output, in which case this bar only prints a
// single status update to output when the current step is updated,
// interspersing the status line with standard program output.  When verbosity
// is not enabled (the default), it is expected that in general output is left
// clean. In this case the status is continually written to standard output
// along with an animated spinner. The status is written one line above current,
// such that any errors from the rest of the process begin at the beinning of a
// line.  Finally, the bar respects headless mode, omitting any printing unless
// explicitly configured to do so, and even then not doing the continual write
// method but the single status update in the same manner as when verbosity is
// enabled.
// Format:
//   [spinner] i/n t
type Bar struct {
	out   io.Writer
	index int    // Current step index
	total int    // Total steps
	text  string // Current display text
	sync.Mutex

	// verbose mode disables progress spinner and line overwrites, instead
	// printing single, full line updates.
	verbose bool

	// print verbose-style updates even when not attached to an interactive terminal.
	printWhileHeadless bool

	// Ticker for animated progress when non-verbose, interactive terminal.
	ticker *time.Ticker
}

type Option func(*Bar)

func WithOutput(w io.Writer) Option {
	return func(b *Bar) {
		b.out = w
	}
}

// WithVerbose indicates the system is in verbose mode, and writing an
// animated line via terminal codes would interrupt the flow of logs.
// When in verbose mode, the bar will print simple status update lines.
func WithVerbose(v bool) Option {
	return func(b *Bar) {
		b.verbose = v
	}
}

// WithPrintHeadless allows for overriding the default behavior of
// squelching all output when the terminal is detected as headless
// (noninteractive)
func WithPrintWhileHeadless(p bool) Option {
	return func(b *Bar) {
		b.printWhileHeadless = p
	}
}

func New(options ...Option) *Bar {
	b := &Bar{
		out: os.Stdout,
	}
	for _, o := range options {
		o(b)
	}
	return b
}

// SetTotal number of steps.
func (b *Bar) SetTotal(n int) {
	b.total = n
}

// Increment the currenly active step, including beginning printing on first call.
func (b *Bar) Increment(text string) {
	b.Lock()
	defer b.Unlock()
	if b.index < b.total {
		b.index++
	}
	b.text = text

	// If this is not an interactive terminal, only print if explicitly set to
	// print while headless, and even then, only a simple line write.
	if !interactiveTerminal() {
		if b.printWhileHeadless {
			b.write()
		}
		return
	}

	// If we're in verbose mode, do a simple write
	if b.verbose {
		b.write()
		return
	}

	// Start the spinner if not already started
	if b.ticker == nil {
		b.ticker = time.NewTicker(100 * time.Millisecond)
		go b.writeOnTick()
	}
}

// Complete the spinner by advancing to the last step, printing the final text and stopping the write loop.
func (b *Bar) Complete(text string) {
	b.Lock()
	defer b.Unlock()
	if !interactiveTerminal() && !b.printWhileHeadless {
		return
	}
	b.index = b.total // Skip to last step
	b.text = text

	// If this is not an interactive terminal, only print if explicitly set to
	// print while headless, and even then, only a simple line write.
	if !interactiveTerminal() {
		if b.printWhileHeadless {
			b.write()
		}
		return
	}

	// If we're interactive, but in verbose mode do a simple write
	if b.verbose {
		b.write()
		return
	}

	// If there is an animated line-overwriting progress indicator running,
	// explicitly stop and then unindent by writing without a spinner prefix.
	if b.ticker != nil {
		b.Done()
		b.overwrite("")
	}
}

// Done cancels the write loop if being used.
// Call in a defer statement after creation to ensure that the bar stops if a
// return is encountered prior to calling the Complete step.
func (b *Bar) Done() {
	if b.ticker != nil {
		b.ticker.Stop()
		b.overwrite("")
	}
}

// Write a simple line status update.
func (b *Bar) write() {
	fmt.Fprintf(b.out, "%v/%v %v\n", b.index, b.total, b.text)
}

// interactiveTerminal returns whether or not the currently attached process
// terminal is interactive.  Used for determining whether or not to
// interactively prompt the user to confirm default choices, etc.
func interactiveTerminal() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && ((fi.Mode() & os.ModeCharDevice) != 0)
}

// Whenever the bar ticks, overwrite the prompt with a bar prefixed by the next
// frame of the spinner.
func (b *Bar) writeOnTick() {
	spinner := []string{"|", "/", "-", "\\"}
	idx := 0
	for _ = range b.ticker.C {
		b.overwrite(spinner[idx] + " ")
		idx = (idx + 1) % 4
	}
}

// overwrite the line prior with the bar text, optional prefix, creating an empty
// current line for potential errors/warnings.
func (b *Bar) overwrite(prefix string) {
	//  1 Move to the front of the current line
	//  2 Move up one line
	//  3 Clear to the end of the line
	//  4 Print status text with optional prefix (spinner)
	//  5 Print linebreak such that subsequent messaes print correctly.
	var (
		up    = "\033[1A"
		clear = "\033[K"
	)
	fmt.Fprintf(b.out, "\r%v%v%v%v/%v %v\n", up, clear, prefix, b.index, b.total, b.text)
}
