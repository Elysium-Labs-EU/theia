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

// tailLog runs "tail" over tailArgs and streams parsed lines to pageViews
// until ctx is canceled or the tail process exits.
//
// A non-nil error means tail exited on its own (e.g. the log file is
// missing, or unreadable due to permissions) rather than being killed by
// ctx cancellation; it wraps tail's own stderr diagnostic so the caller can
// surface a clear, actionable message instead of the daemon silently going
// idle.
func tailLog(ctx context.Context, tailArgs []string, pageViews chan<- PageView) error {
	tailLogCommand := exec.CommandContext(ctx, "tail", tailArgs...) //nolint:gosec // args are internal, not user input

	var stderr bytes.Buffer
	tailLogCommand.Stderr = &stderr

	readCloser, err := tailLogCommand.StdoutPipe()
	if err != nil {
		return fmt.Errorf("setting up log reading: %w", err)
	}

	if err := tailLogCommand.Start(); err != nil {
		if ctx.Err() != nil {
			// ctx was already canceled by the time Start ran (e.g. shutdown
			// landed while Run was still doing DB setup/migrations), so
			// exec.CommandContext refused to start the process at all. That's
			// a graceful shutdown, not a failure to report.
			return nil //nolint:nilerr // shutdown-triggered refusal to start, not a real error
		}
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
	if scanErr := scanner.Err(); scanErr != nil {
		fmt.Printf("error occurred during reading of the log, got: %v\n", scanErr)
	}

	waitErr := tailLogCommand.Wait()
	if ctx.Err() != nil {
		// exec.CommandContext killed the process to honor shutdown, so
		// waitErr (e.g. "signal: killed") is expected, not a failure.
		return nil //nolint:nilerr // shutdown-triggered kill, not a real error
	}
	if waitErr == nil {
		return nil
	}

	stderrMsg := strings.TrimSpace(stderr.String())
	if stderrMsg == "" {
		return fmt.Errorf("tail exited unexpectedly: %w", waitErr)
	}
	return fmt.Errorf("tail exited unexpectedly: %w: %s", waitErr, stderrMsg)
}

// splitLinesSkippingOverlong is a bufio.SplitFunc equivalent to bufio.ScanLines
// except that, instead of aborting the whole scan with bufio.ErrTooLong once a
// line exceeds maxSize, it discards that line and keeps scanning so later
// lines are still ingested. onSkip is invoked once a discarded line's
// terminating newline is found (or at EOF if it never has one), with the
// total number of bytes discarded for that line.
func splitLinesSkippingOverlong(maxSize int, onSkip func(size int)) bufio.SplitFunc {
	s := &overlongLineSplitter{maxSize: maxSize, onSkip: onSkip}
	return s.split
}

// overlongLineSplitter holds the state a bufio.SplitFunc must carry across
// calls: whether it's mid-skip of an overlong line, and how much of that
// line has been discarded so far.
type overlongLineSplitter struct {
	onSkip      func(size int)
	maxSize     int
	skippedSize int
	skipping    bool
}

func (s *overlongLineSplitter) split(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if s.skipping {
		return s.continueSkip(data, atEOF)
	}
	return s.scan(data, atEOF)
}

func (s *overlongLineSplitter) continueSkip(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		s.skipping = false
		s.onSkip(s.skippedSize + i)
		s.skippedSize = 0
		return i + 1, nil, nil
	}
	s.skippedSize += len(data)
	if atEOF {
		s.onSkip(s.skippedSize)
	}
	return len(data), nil, nil
}

func (s *overlongLineSplitter) scan(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		return i + 1, dropCR(data[:i]), nil
	}
	if len(data) >= s.maxSize {
		s.skipping = true
		s.skippedSize = len(data)
		return len(data), nil, nil
	}
	if atEOF {
		return len(data), dropCR(data), nil
	}
	return 0, nil, nil
}

func dropCR(data []byte) []byte {
	if len(data) > 0 && data[len(data)-1] == '\r' {
		return data[:len(data)-1]
	}
	return data
}
