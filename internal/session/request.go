package session

import (
	"fmt"
	"math"

	"kaffeinate/internal/power"
)

type Request struct {
	PowerOptions power.Options `json:"power"`
	WatchedPID   int           `json:"watched_pid,omitempty"`
}

func (request Request) Validate() error {
	if err := request.PowerOptions.Validate(); err != nil {
		return err
	}
	if request.WatchedPID < 0 || request.WatchedPID > math.MaxInt32 {
		return fmt.Errorf("watched PID must be between 1 and %d", math.MaxInt32)
	}
	return nil
}
