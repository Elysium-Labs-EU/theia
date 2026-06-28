package ingest

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
)

func tailLog(ctx context.Context, tailArgs []string, pageViews chan<- PageView) {
	tailLogCommand := exec.CommandContext(ctx, "tail", tailArgs...) //nolint:gosec // args are internal, not user input
	readCloser, err := tailLogCommand.StdoutPipe()
	if err != nil {
		fmt.Printf("error occurred during setting up log reading, got: %v\n", err)
		return
	}

	err = tailLogCommand.Start()
	if err != nil {
		fmt.Printf("error occurred during starting log reading, got: %v\n", err)
		return
	}

	scanner := bufio.NewScanner(readCloser)
	for scanner.Scan() {
		scannerErr := scanner.Err()
		if scannerErr != nil {
			fmt.Printf("error occurred during parsing of the line, got: %v\n", scannerErr)
			continue
		}
		line := scanner.Text()
		pageView, err := parseNginxLog(line)
		if err != nil {
			fmt.Printf("error occurred during parsing of the line, got: %v\n", err)
			continue
		}
		pageViews <- pageView
	}
	_ = tailLogCommand.Wait()
}
