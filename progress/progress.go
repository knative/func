package progress

import (
	"fmt"
	"io"
	"os"
	"runtime"
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

	// verbose mode disables progress spinner and line overwrites, instead
	// printing single, full line updates.
	verbose bool

	// print verbose-style updates even when not attached to an interactive terminal.
	printWhileHeadless bool

	// print N/M step counter with messages
	printWithStepCounter bool

	// Ticker for animated progress when non-verbose, interactive terminal.
	ticker *time.Ticker
}

type Option func(*Bar)

func WithOutput(w io.Writer) Option {
	return func(b *Bar) {
		b.out = w
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

func WithPrintStepCounter(s bool) Option {
	return func(b *Bar) {
		b.printWithStepCounter = s
	}
}

func New(verbose bool, options ...Option) *Bar {
	b := &Bar{
		out:     os.Stdout,
		verbose: verbose,
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
		fmt.Println()
		b.ticker = time.NewTicker(100 * time.Millisecond)
		go b.spin(b.ticker.C)
	}

	// Otherwise we are in non-verbose, interactive mode.  Do a line-overwrite.
	b.overwrite("   ") // Write with space for the spinner
}

// Complete the spinner by advancing to the last step, printing the final text and stopping the write loop.
func (b *Bar) Complete(text string) {
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

	// Otherwise we are in non-verbose mode with an interactive terminal.
	// We should stop the spinner and write an unindented line.
	b.Done() // stop spinner
}

// Stopping indicates the process is stopping, such as having received a context
// cancellation.
func (b *Bar) Stopping() {
	// currently stopping is equivalent in effect to Done
	b.Done()
}

// Done cancels the write loop if being used.
// Call in a defer statement after creation to ensure that the spinner stops
func (b *Bar) Done() {
	if b.ticker != nil {
		b.ticker.Stop()
		b.ticker = nil
	}
}

// Write a simple line status update.
func (b *Bar) write() {
	fmt.Fprintln(b.out, b)
}

// interactiveTerminal returns whether or not the currently attached process
// terminal is interactive.  Used for determining whether or not to
// interactively prompt the user to confirm default choices, etc.
func interactiveTerminal() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && ((fi.Mode() & os.ModeCharDevice) != 0)
}

const (
	up    = "\033[1A"
	down  = "\033[1B"
	clear = "\033[K"
)

// overwrite the line prior with the bar text, optional prefix, creating an empty
// current line for potential errors/warnings.
func (b *Bar) overwrite(prefix string) {
	//  1 Move to the front of the current line
	//  2 Move up one line
	//  3 Clear to the end of the line
	//  4 Print status text with optional prefix (spinner)
	//  5 Print linebreak such that subsequent messages print correctly.
	if runtime.GOOS == "windows" {
		fmt.Fprintf(b.out, "\r%v%v\n", prefix, b)
	} else {
		fmt.Fprintf(b.out, "\r%v%v%v%v\n", up, clear, prefix, b)
	}
}

func (b *Bar) String() string {
	if b.printWithStepCounter {
		return fmt.Sprintf("%v/%v %v", b.index, b.total, b.text)
	}
	return b.text
}

// Write a spinner at the beginning of the previous line.
func (b *Bar) spin(ch <-chan time.Time) {
	if b.verbose {
		return
	}
	// Various options for spinners.
	// spinner := []string{"|", "/", "-", "\\"}
	// spinner := []string{"◢", "◣", "◤", "◥"}
	spinner := []string{
		"🕛 ",
		"🕐 ",
		"🕑 ",
		"🕒 ",
		"🕓 ",
		"🕔 ",
		"🕕 ",
		"🕖 ",
		"🕗 ",
		"🕘 ",
		"🕙 ",
		"🕚 ",
	}
	if runtime.GOOS == "windows" {
		spinner = []string{"|", "/", "-", "\\"}
	}
	idx := 0
	for range ch {
		// Writes the spinner frame at the beginning of the previous line, moving
		// the cursor back to the beginning of the current line for any errors or
		// informative messages.
		if runtime.GOOS == "windows" {
			fmt.Fprintf(b.out, "\r%v\r", spinner[idx])
		} else {
			fmt.Fprintf(b.out, "\r%v%v%v\r", up, spinner[idx], down)
		}
		idx = (idx + 1) % len(spinner)
	}
}
