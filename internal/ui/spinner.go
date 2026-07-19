package ui

import (
	"fmt"
	"os"
	"time"

	"github.com/mattn/go-isatty"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// WithSpinner runs fn, showing an indeterminate spinner with an elapsed
// timer on stderr for the duration. Update downloads and checksum work
// don't report discrete progress, so a spinner (not a percentage bar) is
// the honest signal that theia hasn't hung.
//
// The spinner is suppressed when stderr isn't a terminal (piped output,
// systemd-invoked runs, CI logs), since the carriage-return redraw only
// makes sense on an interactive terminal.
func WithSpinner(message string, fn func() error) error {
	if !isatty.IsTerminal(os.Stderr.Fd()) {
		return fn()
	}

	done := make(chan error, 1)
	go func() { done <- fn() }()

	start := time.Now()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	frame := 0
	for {
		select {
		case err := <-done:
			fmt.Fprint(os.Stderr, "\r\033[K")
			return err
		case <-ticker.C:
			elapsed := time.Since(start).Round(time.Second)
			fmt.Fprintf(os.Stderr, "\r\033[K%s %s %s",
				LabelInfo.Render(spinnerFrames[frame%len(spinnerFrames)]),
				message,
				TextMuted.Render(elapsed.String()))
			frame++
		}
	}
}
