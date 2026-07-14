package ui

import "fmt"

// UserError is an error meant for direct display to a human: a concise
// message plus an optional actionable next step. Wrap errors in it at the
// point where a raw Go error chain (exec.ErrNotFound, os.PathError, ...)
// would otherwise leak internals to the terminal.
type UserError struct {
	Err  error
	Hint string
}

func (e *UserError) Error() string { return e.Err.Error() }
func (e *UserError) Unwrap() error { return e.Err }

// Render formats the error for stderr: a bold red "error" label with the
// message, and a muted "run: <command>" hint line when one is set.
func (e *UserError) Render() string {
	out := fmt.Sprintf("%s %s", LabelError.Render("error"), e.Error())
	if e.Hint != "" {
		out += fmt.Sprintf("\n  %s %s", TextMuted.Render("run:"), TextCommand.Render(e.Hint))
	}
	return out
}
