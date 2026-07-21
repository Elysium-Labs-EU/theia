package ingest

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// maxLogLineSize bounds how much of a single log line tailLog will buffer.
// Access-log lines carry attacker-controlled fields (URL, Referer, User-Agent),
// so an external visitor can otherwise send an arbitrarily long line; the cap
// keeps memory use bounded while staying generous enough for realistic traffic.
const maxLogLineSize = 1 << 20 // 1 MiB

func tailLog(ctx context.Context, tailArgs []string, pageViews chan<- PageView) error {
	tailLogCommand := exec.CommandContext(ctx, "tail", tailArgs...) //nolint:gosec // args are internal, not user input

	readCloser, err := tailLogCommand.StdoutPipe()
	if err != nil {
		return fmt.Errorf("setting up log reading: %w", err)
	}

	var stderr bytes.Buffer
	tailLogCommand.Stderr = &stderr

	if err := tailLogCommand.Start(); err != nil {
		return fmt.Errorf("starting log reading: %w", err)
	}

	scanner := bufio.NewScanner(readCloser)
	scanner.Buffer(make([]byte, 64*1024), maxLogLineSize*2)
	scanner.Split(splitLinesSkippingOverlong(maxLogLineSize, func(size int) {
		fmt.Printf("skipping log line of %d bytes, exceeds %d byte limit; ingestion continues\n", size, maxLogLineSize)
	}))

	for scanner.Scan() {
		line := scanner.Text()
		pageView, err := parseNginxLog(line)
		if err != nil {
			fmt.Printf("error occurred during parsing of the line, got: %v\n", err)
			continue
		}
		pageViews <- pageView
	}
	scanErr := scanner.Err()

	if waitErr := tailLogCommand.Wait(); waitErr != nil {
		if ctx.Err() != nil {
			// Caller-requested shutdown (e.g. SIGINT/SIGTERM) killed the tail
			// child via exec.CommandContext; that's a clean stop, not a failure.
			return nil
		}
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("tail command failed: %w: %s", waitErr, msg)
		}
		return fmt.Errorf("tail command failed: %w", waitErr)
	}
	if scanErr != nil {
		return fmt.Errorf("reading tail output: %w", scanErr)
	}

	return nil
}

// splitLinesSkippingOverlong is a bufio.SplitFunc equivalent to bufio.ScanLines
// except that, instead of aborting the whole scan with bufio.ErrTooLong once a
// line exceeds maxSize, it discards that line and keeps scanning so later
// lines are still ingested. onSkip is invoked once a discarded line's
// terminating newline is found (or at EOF if it never has one), with the
// total number of bytes discarded for that line.
func splitLinesSkippingOverlong(maxSize int, onSkip func(size int)) bufio.SplitFunc {
	skipping := false
	skippedSize := 0

	return func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}

		if skipping {
			if i := bytes.IndexByte(data, '\n'); i >= 0 {
				skipping = false
				onSkip(skippedSize + i)
				skippedSize = 0
				return i + 1, nil, nil
			}
			skippedSize += len(data)
			if atEOF {
				onSkip(skippedSize)
			}
			return len(data), nil, nil
		}

		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			return i + 1, dropCR(data[:i]), nil
		}

		if len(data) >= maxSize {
			skipping = true
			skippedSize = len(data)
			return len(data), nil, nil
		}

		if atEOF {
			return len(data), dropCR(data), nil
		}

		return 0, nil, nil
	}
}

func dropCR(data []byte) []byte {
	if len(data) > 0 && data[len(data)-1] == '\r' {
		return data[:len(data)-1]
	}
	return data
}
