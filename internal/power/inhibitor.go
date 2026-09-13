package power

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Behaviors uint8

const (
	IdleSystemSleep Behaviors = 1 << iota
	DisplaySleep
	DiskIdleSleep
	ACSystemSleep
	UserActivity
	AllBehaviors = IdleSystemSleep | DisplaySleep | DiskIdleSleep | ACSystemSleep | UserActivity
)

func (behaviors Behaviors) String() string {
	var descriptions []string
	for _, behavior := range []struct {
		value Behaviors
		label string
	}{
		{IdleSystemSleep, "idle system sleep prevented"},
		{DisplaySleep, "display kept awake"},
		{DiskIdleSleep, "disk idle sleep prevented"},
		{ACSystemSleep, "system sleep prevented on AC power"},
		{UserActivity, "user activity asserted"},
	} {
		if behaviors&behavior.value != 0 {
			descriptions = append(descriptions, behavior.label)
		}
	}
	return strings.Join(descriptions, "; ")
}

type Options struct {
	Behaviors Behaviors     `json:"behaviors"`
	Duration  time.Duration `json:"duration"`
}

func (options Options) Validate() error {
	if options.Behaviors == 0 || options.Behaviors&^AllBehaviors != 0 {
		return fmt.Errorf("choose at least one supported power behavior")
	}
	if options.Duration < 0 || options.Duration%time.Second != 0 {
		return fmt.Errorf("duration must be a nonnegative whole number of seconds")
	}
	return nil
}

type Assertion struct {
	Behavior  Behaviors
	ExpiresAt time.Time
}

type Hold interface {
	Assertions() []Assertion
	Done() <-chan struct{}
	Err() error
	Release() error
}

type Watch interface {
	Done() <-chan struct{}
	Err() error
	Close() error
}

type Backend interface {
	Acquire(context.Context, Options) (Hold, error)
	WatchProcess(int) (Watch, error)
}
