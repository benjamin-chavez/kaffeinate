package tray_test

import (
	"strings"
	"testing"
	"time"

	"kaffeinate/internal/power"
	"kaffeinate/internal/session"
	"kaffeinate/internal/tray"
)

func TestSessionPresentation(t *testing.T) {
	t.Run("when a CLI session is timed, shows its mode and end time", func(t *testing.T) {
		deadline := time.Date(2026, 9, 12, 2, 30, 0, 0, time.Local)
		presentation := tray.Present(session.Snapshot{Status: session.Active, Behaviors: power.DisplaySleep, Deadline: deadline, WatchedPID: 123})
		if !presentation.Active || presentation.Busy || !strings.Contains(presentation.StatusText, "Sep 12, 2:30 AM") || presentation.BehaviorText != "display kept awake" || presentation.ProcessText != "Until PID 123 exits" {
			t.Fatalf("presentation=%+v", presentation)
		}
	})
	t.Run("when an AC-only request is active, states its condition", func(t *testing.T) {
		presentation := tray.Present(session.Snapshot{Status: session.Active, Behaviors: power.ACSystemSleep})
		if !strings.Contains(presentation.BehaviorText, "on AC power") || presentation.StatusText != "Active indefinitely" {
			t.Fatalf("presentation=%+v", presentation)
		}
	})
	t.Run("when starting or stopping, disables conflicting controls", func(t *testing.T) {
		for _, status := range []session.Status{session.Starting, session.Stopping} {
			if presentation := tray.Present(session.Snapshot{Status: status}); !presentation.Busy {
				t.Fatalf("presentation=%+v", presentation)
			}
		}
	})
	t.Run("when activation failed, shows the error and allows retry", func(t *testing.T) {
		presentation := tray.Present(session.Snapshot{Status: session.Inactive, Error: "power request failed"})
		if presentation.Active || presentation.Busy || presentation.ErrorText != "Error: power request failed" {
			t.Fatalf("presentation=%+v", presentation)
		}
	})
}
