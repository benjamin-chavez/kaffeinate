package session

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"kaffeinate/internal/power"
)

var ErrClosed = errors.New("Kaffeinate is shutting down")

type Status string

const (
	Inactive Status = "inactive"
	Starting Status = "starting"
	Active   Status = "active"
	Stopping Status = "stopping"
	Closed   Status = "closed"
)

type Snapshot struct {
	Status     Status          `json:"status"`
	Behaviors  power.Behaviors `json:"behaviors"`
	Deadline   time.Time       `json:"deadline,omitempty"`
	WatchedPID int             `json:"watched_pid,omitempty"`
	Error      string          `json:"error,omitempty"`
}

type Controller struct {
	backend   power.Backend
	commands  chan command
	changes   chan struct{}
	done      chan struct{}
	snapshot  atomic.Value
	closeOnce sync.Once
	closeErr  error
}

type command struct {
	kind    Status
	ctx     context.Context
	request Request
	result  chan commandResult
}

type commandResult struct {
	snapshot Snapshot
	err      error
}

type activeSession struct {
	hold          power.Hold
	watch         power.Watch
	assertions    []power.Assertion
	request       Request
	cleanupFailed bool
}

func NewController(backend power.Backend) *Controller {
	controller := &Controller{
		backend: backend, commands: make(chan command), changes: make(chan struct{}, 1), done: make(chan struct{}),
	}
	controller.snapshot.Store(Snapshot{Status: Inactive})
	go controller.run()
	return controller
}

func (controller *Controller) Current() Snapshot { return controller.snapshot.Load().(Snapshot) }

// Changes signals that Current has changed. A slow receiver always has access to the latest snapshot.
func (controller *Controller) Changes() <-chan struct{} { return controller.changes }

func (controller *Controller) Start(ctx context.Context, request Request) (Snapshot, error) {
	if err := request.Validate(); err != nil {
		return controller.Current(), err
	}
	return controller.submit(ctx, Starting, request)
}

func (controller *Controller) Stop(ctx context.Context) (Snapshot, error) {
	return controller.submit(ctx, Stopping, Request{})
}

func (controller *Controller) Close() error {
	controller.closeOnce.Do(func() {
		_, controller.closeErr = controller.submit(context.Background(), Closed, Request{})
	})
	return controller.closeErr
}

func (controller *Controller) submit(ctx context.Context, kind Status, request Request) (Snapshot, error) {
	result := make(chan commandResult, 1)
	select {
	case <-ctx.Done():
		return controller.Current(), ctx.Err()
	case <-controller.done:
		return controller.Current(), ErrClosed
	case controller.commands <- command{kind: kind, ctx: ctx, request: request, result: result}:
	}
	select {
	case response := <-result:
		return response.snapshot, response.err
	case <-ctx.Done():
		return controller.Current(), ctx.Err()
	}
}

func (controller *Controller) publish(snapshot Snapshot) {
	controller.snapshot.Store(snapshot)
	select {
	case controller.changes <- struct{}{}:
	default:
	}
}

func (controller *Controller) run() {
	var currentSession *activeSession
	expiryTimer := time.NewTimer(time.Hour)
	expiryTimer.Stop()
	defer expiryTimer.Stop()
	defer close(controller.done)
	defer close(controller.changes)
	for {
		var holdDone, watchDone <-chan struct{}
		var expiry <-chan time.Time
		if currentSession != nil && !currentSession.cleanupFailed {
			holdDone = currentSession.hold.Done()
			if currentSession.watch != nil {
				watchDone = currentSession.watch.Done()
			}
			if nextExpiry := currentSession.nextExpiry(); !nextExpiry.IsZero() {
				delay := min(time.Second, nextExpiry.Sub(time.Now().Round(0)))
				expiryTimer.Reset(max(0, delay))
				expiry = expiryTimer.C
			}
		}
		select {
		case nextCommand := <-controller.commands:
			if err := nextCommand.ctx.Err(); err != nil {
				nextCommand.result <- commandResult{controller.Current(), err}
				continue
			}
			var err error
			switch nextCommand.kind {
			case Starting:
				currentSession, err = controller.start(nextCommand.ctx, currentSession, nextCommand.request)
			case Stopping, Closed:
				currentSession, err = controller.stop(currentSession, nil)
				if nextCommand.kind == Closed {
					if currentSession != nil && currentSession.watch != nil {
						err = errors.Join(err, currentSession.watch.Close())
					}
					snapshot := controller.Current()
					snapshot.Status = Closed
					if err != nil {
						snapshot.Error = err.Error()
					}
					controller.publish(snapshot)
					nextCommand.result <- commandResult{snapshot, err}
					return
				}
			}
			nextCommand.result <- commandResult{controller.Current(), err}
		case <-expiry:
			currentSession = controller.expire(currentSession)
		case <-watchDone:
			currentSession, _ = controller.stop(currentSession, currentSession.watch.Err())
		case <-holdDone:
			currentSession = controller.expire(currentSession)
			if currentSession != nil {
				exitErr := currentSession.hold.Err()
				if exitErr == nil {
					exitErr = errors.New("sleep prevention ended unexpectedly; start a new session to retry")
				}
				currentSession, _ = controller.stop(currentSession, exitErr)
			}
		}
		expiryTimer.Stop()
	}
}

func (controller *Controller) start(ctx context.Context, previousSession *activeSession, request Request) (*activeSession, error) {
	var watchedProcess power.Watch
	if request.WatchedPID != 0 {
		var err error
		watchedProcess, err = controller.backend.WatchProcess(request.WatchedPID)
		if err != nil {
			snapshot := controller.Current()
			snapshot.Error = err.Error()
			controller.publish(snapshot)
			return previousSession, err
		}
	}
	remainingSession, err := controller.stop(previousSession, nil)
	if err != nil {
		if watchedProcess != nil {
			_ = watchedProcess.Close()
		}
		return remainingSession, err
	}
	if watchedProcess != nil {
		select {
		case <-watchedProcess.Done():
			err := watchedProcess.Close()
			controller.publish(inactiveSnapshot(err))
			return nil, err
		default:
		}
	}
	controller.publish(Snapshot{Status: Starting})
	startedAt := time.Now().Round(0)
	hold, err := controller.backend.Acquire(ctx, request.PowerOptions)
	if err != nil {
		if watchedProcess != nil {
			_ = watchedProcess.Close()
		}
		controller.publish(inactiveSnapshot(err))
		return nil, err
	}
	current := &activeSession{hold: hold, watch: watchedProcess, assertions: hold.Assertions(), request: request}
	if request.PowerOptions.Duration > 0 {
		deadline := startedAt.Add(request.PowerOptions.Duration)
		for index := range current.assertions {
			assertion := &current.assertions[index]
			if assertion.ExpiresAt.IsZero() || assertion.ExpiresAt.After(deadline) {
				assertion.ExpiresAt = deadline
			}
		}
	}
	controller.publish(current.snapshot())
	return controller.expire(current), nil
}

func (controller *Controller) stop(current *activeSession, cause error) (*activeSession, error) {
	if current != nil {
		snapshot := controller.Current()
		snapshot.Status = Stopping
		controller.publish(snapshot)
		if err := current.hold.Release(); err != nil {
			current.cleanupFailed = true
			snapshot.Status = Active
			snapshot.Error = fmt.Sprintf("could not release sleep prevention: %v", err)
			controller.publish(snapshot)
			return current, err
		}
		if current.watch != nil {
			cause = errors.Join(cause, current.watch.Close())
		}
	}
	controller.publish(inactiveSnapshot(cause))
	return nil, cause
}

func (controller *Controller) expire(current *activeSession) *activeSession {
	if current == nil {
		return nil
	}
	select {
	case <-current.hold.Done():
		if err := current.hold.Err(); err != nil {
			current, _ = controller.stop(current, err)
			return current
		}
	default:
	}
	now := time.Now().Round(0)
	remainingAssertions := current.assertions[:0]
	for _, assertion := range current.assertions {
		if assertion.ExpiresAt.IsZero() || assertion.ExpiresAt.After(now) {
			remainingAssertions = append(remainingAssertions, assertion)
		}
	}
	if len(remainingAssertions) == len(current.assertions) {
		return current
	}
	current.assertions = remainingAssertions
	if len(remainingAssertions) == 0 {
		current, _ = controller.stop(current, nil)
		return current
	}
	controller.publish(current.snapshot())
	return current
}

func (current *activeSession) nextExpiry() time.Time {
	var nextExpiry time.Time
	for _, assertion := range current.assertions {
		if !assertion.ExpiresAt.IsZero() && (nextExpiry.IsZero() || assertion.ExpiresAt.Before(nextExpiry)) {
			nextExpiry = assertion.ExpiresAt
		}
	}
	return nextExpiry
}

func (current *activeSession) snapshot() Snapshot {
	snapshot := Snapshot{Status: Active, WatchedPID: current.request.WatchedPID}
	indefinite := false
	for _, assertion := range current.assertions {
		snapshot.Behaviors |= assertion.Behavior
		if assertion.ExpiresAt.IsZero() {
			indefinite = true
		} else if assertion.ExpiresAt.After(snapshot.Deadline) {
			snapshot.Deadline = assertion.ExpiresAt
		}
	}
	if indefinite {
		snapshot.Deadline = time.Time{}
	}
	return snapshot
}

func inactiveSnapshot(err error) Snapshot {
	snapshot := Snapshot{Status: Inactive}
	if err != nil {
		snapshot.Error = err.Error()
	}
	return snapshot
}
