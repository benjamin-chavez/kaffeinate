package tray

import (
	"fmt"

	"kaffeinate/internal/power"
	"kaffeinate/internal/session"
)

type Presentation struct {
	StatusText   string
	BehaviorText string
	ProcessText  string
	ErrorText    string
	Active       bool
	Busy         bool
}

func Present(snapshot session.Snapshot) Presentation {
	presentation := Presentation{
		StatusText: "Kaffeinate: Inactive",
		Active:     snapshot.Behaviors != 0,
		Busy:       snapshot.Status == session.Starting || snapshot.Status == session.Stopping || snapshot.Status == session.Closed,
	}
	switch snapshot.Status {
	case session.Starting:
		presentation.StatusText = "Starting session…"
	case session.Stopping:
		presentation.StatusText = "Stopping session…"
	case session.Closed:
		presentation.StatusText = "Quitting Kaffeinate…"
	case session.Active:
		prefix := "Active"
		if snapshot.Behaviors&power.IdleSystemSleep != 0 {
			prefix = "Awake"
		}
		presentation.StatusText = prefix + " indefinitely"
		if !snapshot.Deadline.IsZero() {
			presentation.StatusText = prefix + " until " + snapshot.Deadline.Local().Format("Jan 2, 3:04 PM")
		}
	}
	if presentation.Active {
		presentation.BehaviorText = snapshot.Behaviors.String()
		if snapshot.WatchedPID != 0 {
			presentation.ProcessText = fmt.Sprintf("Until PID %d exits", snapshot.WatchedPID)
		}
	}
	if snapshot.Error != "" {
		errorRunes := []rune(snapshot.Error)
		if len(errorRunes) > 120 {
			errorRunes = append(errorRunes[:117], '…')
		}
		presentation.ErrorText = "Error: " + string(errorRunes)
	}
	return presentation
}
