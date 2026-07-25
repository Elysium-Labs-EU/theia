package ui

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Confirm prints a yes/no prompt to out and reads one line of response from
// in. An empty response (bare Enter) resolves to defaultYes.
//
// in must be shared across calls (not re-wrapped per call) when prompting
// more than once against the same underlying stream: bufio.Reader reads
// ahead in chunks, so a fresh reader per call can buffer past the first
// answer and silently consume input meant for a later prompt.
func Confirm(in *bufio.Reader, out io.Writer, prompt string, defaultYes bool) bool {
	suffix := "[y/N]"
	if defaultYes {
		suffix = "[Y/n]"
	}
	_, _ = fmt.Fprintf(out, "%s %s %s ", LabelWarning.Render("?"), prompt, suffix)

	line, _ := in.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return defaultYes
	}
	return line == "y" || line == "yes"
}
