package ui

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Confirm prints a yes/no prompt to out and reads one line of response from
// in. An empty response (bare Enter) resolves to defaultYes.
func Confirm(in io.Reader, out io.Writer, prompt string, defaultYes bool) bool {
	suffix := "[y/N]"
	if defaultYes {
		suffix = "[Y/n]"
	}
	_, _ = fmt.Fprintf(out, "%s %s %s ", LabelWarning.Render("?"), prompt, suffix)

	line, _ := bufio.NewReader(in).ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return defaultYes
	}
	return line == "y" || line == "yes"
}
