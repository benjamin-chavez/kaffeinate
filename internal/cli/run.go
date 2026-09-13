package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"kaffeinate/internal/control"
	"kaffeinate/internal/session"
)

type Runner struct {
	Client control.Client
	Launch func(context.Context) error
}

func (runner Runner) Run(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	request, help, err := Parse(arguments)
	if err != nil {
		fmt.Fprintf(stderr, "kaffeinate: %v\n", err)
		return 2
	}
	if help {
		fmt.Fprint(stdout, Help)
		return 0
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	snapshot, err := runner.Client.Start(ctx, request)
	if errors.Is(err, control.ErrUnavailable) {
		if err = runner.Launch(ctx); err == nil {
			snapshot, err = runner.awaitApplication(ctx, request)
		}
	}
	if err != nil {
		fmt.Fprintf(stderr, "kaffeinate: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, Confirmation(snapshot))
	return 0
}

func (runner Runner) awaitApplication(ctx context.Context, request session.Request) (session.Snapshot, error) {
	readinessTicker := time.NewTicker(50 * time.Millisecond)
	defer readinessTicker.Stop()
	for {
		snapshot, err := runner.Client.Start(ctx, request)
		if !errors.Is(err, control.ErrUnavailable) {
			return snapshot, err
		}
		select {
		case <-ctx.Done():
			return session.Snapshot{}, fmt.Errorf("application did not become ready: %w", ctx.Err())
		case <-readinessTicker.C:
		}
	}
}

func Confirmation(snapshot session.Snapshot) string {
	if snapshot.Status == session.Inactive {
		return "Kaffeinate: session already ended."
	}
	duration := "indefinitely"
	if !snapshot.Deadline.IsZero() {
		duration = "until " + snapshot.Deadline.Local().Format("Jan 2, 3:04:05 PM MST")
	}
	message := fmt.Sprintf("Kaffeinate: %s %s", snapshot.Behaviors, duration)
	if snapshot.WatchedPID != 0 {
		message += fmt.Sprintf(" or until PID %d exits", snapshot.WatchedPID)
		return message + ". The session follows the watched process's lifetime."
	}
	return message + ". You can close this terminal."
}
