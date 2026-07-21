package ingest

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

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
