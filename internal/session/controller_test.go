package session_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"kaffeinate/internal/power"
	"kaffeinate/internal/session"
)

func TestSessionStart(t *testing.T) {
	t.Run("when inactive, starts an indefinite session", func(t *testing.T) {
		backend, controller := newController(t)
		snapshot, err := controller.Start(context.Background(), idleRequest(0))
		if err != nil || snapshot.Status != session.Active || !snapshot.Deadline.IsZero() || backend.activeCount() != 1 {
			t.Fatalf("snapshot=%+v error=%v active=%d", snapshot, err, backend.activeCount())
		}
	})
	t.Run("when activation fails, remains inactive", func(t *testing.T) {
		backend, controller := newController(t)
		backend.acquireErr = errors.New("power unavailable")
		snapshot, err := controller.Start(context.Background(), idleRequest(0))
		if err == nil || snapshot.Status != session.Inactive || snapshot.Error == "" || backend.activeCount() != 0 {
			t.Fatalf("snapshot=%+v error=%v", snapshot, err)
		}
	})
	t.Run("when canceled before admission, stays inactive", func(t *testing.T) {
		backend, controller := newController(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := controller.Start(ctx, idleRequest(0))
		if !errors.Is(err, context.Canceled) || backend.activeCount() != 0 {
			t.Fatalf("error=%v active=%d", err, backend.activeCount())
		}
	})
}

func TestSessionNotifications(t *testing.T) {
	t.Run("when a session starts and expires, notifies its observer", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			_, controller := newController(t)
			observedStates := make(chan session.Snapshot, 8)
			go func() {
				defer close(observedStates)
				for range controller.Changes() {
					select {
					case observedStates <- controller.Current():
					default:
					}
				}
			}()
			awaitStatus := func(status session.Status) {
				t.Helper()
				deadline := time.NewTimer(time.Second)
				defer deadline.Stop()
				for {
					select {
					case snapshot, ok := <-observedStates:
						if !ok {
							t.Fatal("observer closed before the expected state")
						}
						if snapshot.Status == status {
							return
						}
					case <-deadline.C:
						t.Fatalf("observer was not notified of %s", status)
					}
				}
			}
			start(t, controller, idleRequest(10*time.Second))
			awaitStatus(session.Active)
			time.Sleep(10 * time.Second)
			awaitStatus(session.Inactive)
		})
	})
}

func TestSessionStop(t *testing.T) {
	for _, active := range []bool{false, true} {
		name := "when already inactive, succeeds"
		if active {
			name = "when active, releases sleep prevention"
		}
		t.Run(name, func(t *testing.T) {
			backend, controller := newController(t)
			if active {
				start(t, controller, idleRequest(0))
			}
			for range 2 {
				snapshot, err := controller.Stop(context.Background())
				if err != nil || snapshot.Status != session.Inactive || backend.activeCount() != 0 {
					t.Fatalf("snapshot=%+v error=%v active=%d", snapshot, err, backend.activeCount())
				}
			}
		})
	}
}

func TestSessionReplacement(t *testing.T) {
	t.Run("when input is invalid, preserves the current session", func(t *testing.T) {
		backend, controller := newController(t)
		before := start(t, controller, idleRequest(time.Hour))
		after, err := controller.Start(context.Background(), session.Request{})
		if err == nil || before != after || backend.activeCount() != 1 {
			t.Fatalf("before=%+v after=%+v error=%v", before, after, err)
		}
	})
	t.Run("when acquisition fails, releases the previous session", func(t *testing.T) {
		backend, controller := newController(t)
		start(t, controller, idleRequest(0))
		backend.mu.Lock()
		backend.acquireErr = errors.New("new request failed")
		backend.mu.Unlock()
		snapshot, err := controller.Start(context.Background(), idleRequest(time.Hour))
		if err == nil || snapshot.Status != session.Inactive || backend.activeCount() != 0 {
			t.Fatalf("snapshot=%+v error=%v active=%d", snapshot, err, backend.activeCount())
		}
	})
	t.Run("when release fails, keeps ownership without overlap", func(t *testing.T) {
		backend, controller := newController(t)
		start(t, controller, idleRequest(0))
		currentHold := backend.latestHold()
		currentHold.setReleaseError(errors.New("cannot release"))
		defer currentHold.setReleaseError(nil)
		snapshot, err := controller.Start(context.Background(), idleRequest(time.Hour))
		if err == nil || snapshot.Status != session.Active || snapshot.Error == "" || backend.activeCount() != 1 {
			t.Fatalf("snapshot=%+v error=%v active=%d", snapshot, err, backend.activeCount())
		}
	})
	t.Run("when the old deadline passes, keeps the new session active", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			backend, controller := newController(t)
			start(t, controller, idleRequest(10*time.Second))
			oldHold := backend.latestHold()
			time.Sleep(5 * time.Second)
			newSnapshot := start(t, controller, idleRequest(20*time.Second))
			oldHold.complete(errors.New("late old exit"))
			time.Sleep(6 * time.Second)
			synctest.Wait()
			if controller.Current().Status != session.Active || controller.Current().Deadline != newSnapshot.Deadline || backend.activeCount() != 1 {
				t.Fatalf("current=%+v active=%d", controller.Current(), backend.activeCount())
			}
			time.Sleep(14 * time.Second)
			synctest.Wait()
			if controller.Current().Status != session.Inactive || backend.activeCount() != 0 {
				t.Fatalf("current=%+v active=%d", controller.Current(), backend.activeCount())
			}
		})
	})
}

func TestSessionExpiry(t *testing.T) {
	t.Run("when a reported deadline has passed, ends immediately", func(t *testing.T) {
		backend, controller := newController(t)
		backend.lifetimes[power.UserActivity] = -time.Second
		start(t, controller, session.Request{PowerOptions: power.Options{Behaviors: power.UserActivity}})
		if snapshot := controller.Current(); snapshot.Status != session.Inactive || backend.activeCount() != 0 {
			t.Fatalf("current=%+v active=%d", snapshot, backend.activeCount())
		}
	})
	t.Run("when the deadline passes, releases sleep prevention", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			backend, controller := newController(t)
			start(t, controller, idleRequest(30*time.Second))
			time.Sleep(30 * time.Second)
			synctest.Wait()
			if controller.Current().Status != session.Inactive || backend.activeCount() != 0 {
				t.Fatalf("current=%+v active=%d", controller.Current(), backend.activeCount())
			}
		})
	})
	t.Run("when short activity expires, retains the longer request", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			backend, controller := newController(t)
			backend.lifetimes[power.UserActivity] = 5 * time.Second
			request := idleRequest(0)
			request.PowerOptions.Behaviors |= power.UserActivity
			start(t, controller, request)
			time.Sleep(5 * time.Second)
			synctest.Wait()
			if snapshot := controller.Current(); snapshot.Status != session.Active || snapshot.Behaviors != power.IdleSystemSleep || !snapshot.Deadline.IsZero() {
				t.Fatalf("current=%+v", snapshot)
			}
		})
	})
	t.Run("when all assertions expire, ends the session", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			backend, controller := newController(t)
			backend.lifetimes[power.UserActivity] = 5 * time.Second
			request := session.Request{PowerOptions: power.Options{Behaviors: power.UserActivity}}
			start(t, controller, request)
			time.Sleep(5 * time.Second)
			synctest.Wait()
			if controller.Current().Status != session.Inactive || backend.activeCount() != 0 {
				t.Fatalf("current=%+v", controller.Current())
			}
		})
	})
	t.Run("when AC power is required, preserves the conditional request", func(t *testing.T) {
		_, controller := newController(t)
		snapshot := start(t, controller, session.Request{PowerOptions: power.Options{Behaviors: power.ACSystemSleep}})
		if snapshot.Behaviors != power.ACSystemSleep || !snapshot.Deadline.IsZero() {
			t.Fatalf("current=%+v", snapshot)
		}
	})
}

func TestSessionProcessWatch(t *testing.T) {
	t.Run("when registration fails, preserves the active session", func(t *testing.T) {
		backend, controller := newController(t)
		before := start(t, controller, idleRequest(time.Hour))
		backend.watchErr = errors.New("process cannot be watched")
		request := idleRequest(0)
		request.WatchedPID = 123
		after, err := controller.Start(context.Background(), request)
		if err == nil || after.Status != session.Active || after.Deadline != before.Deadline || backend.activeCount() != 1 {
			t.Fatalf("before=%+v after=%+v error=%v", before, after, err)
		}
	})
	t.Run("when release fails after PID exit, remains responsive", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			backend, controller := newController(t)
			request := idleRequest(0)
			request.WatchedPID = 123
			start(t, controller, request)
			hold := backend.latestHold()
			hold.setReleaseError(errors.New("release unavailable"))
			backend.processWatch.complete(nil)
			synctest.Wait()
			if snapshot := controller.Current(); snapshot.Status != session.Active || snapshot.Error == "" {
				t.Fatalf("current=%+v", snapshot)
			}
			hold.setReleaseError(nil)
			if _, err := controller.Stop(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	})
	for _, pidFirst := range []bool{true, false} {
		name := "when timeout arrives first, ends the session"
		if pidFirst {
			name = "when the watched PID exits first, ends the session"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				backend, controller := newController(t)
				request := idleRequest(10 * time.Second)
				request.WatchedPID = 123
				start(t, controller, request)
				if pidFirst {
					backend.processWatch.complete(nil)
				} else {
					time.Sleep(10 * time.Second)
				}
				synctest.Wait()
				if controller.Current().Status != session.Inactive || backend.activeCount() != 0 {
					t.Fatalf("current=%+v", controller.Current())
				}
			})
		})
	}
	t.Run("when PID has already exited, remains inactive", func(t *testing.T) {
		backend, controller := newController(t)
		backend.processWatch.complete(nil)
		request := idleRequest(0)
		request.WatchedPID = 123
		snapshot := start(t, controller, request)
		if snapshot.Status != session.Inactive || backend.activeCount() != 0 {
			t.Fatalf("current=%+v", snapshot)
		}
	})
}

func TestSessionFailure(t *testing.T) {
	t.Run("when a later start succeeds, clears the previous error", func(t *testing.T) {
		backend, controller := newController(t)
		backend.acquireErr = errors.New("temporarily unavailable")
		if _, err := controller.Start(context.Background(), idleRequest(0)); err == nil {
			t.Fatal("expected activation failure")
		}
		backend.mu.Lock()
		backend.acquireErr = nil
		backend.mu.Unlock()
		snapshot := start(t, controller, idleRequest(0))
		if snapshot.Status != session.Active || snapshot.Error != "" || backend.activeCount() != 1 {
			t.Fatalf("recovered session=%+v", snapshot)
		}
	})
	t.Run("when timed failure cannot be released, allows a later retry", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			backend, controller := newController(t)
			start(t, controller, idleRequest(10*time.Second))
			hold := backend.latestHold()
			hold.setReleaseError(errors.New("release unavailable"))
			hold.complete(errors.New("native failure"))
			time.Sleep(11 * time.Second)
			synctest.Wait()
			if snapshot := controller.Current(); snapshot.Status != session.Active || snapshot.Error == "" {
				t.Fatalf("current=%+v", snapshot)
			}
			hold.setReleaseError(nil)
			if _, err := controller.Stop(context.Background()); err != nil || backend.activeCount() != 0 {
				t.Fatalf("retry error=%v active=%d", err, backend.activeCount())
			}
		})
	})
	t.Run("when failure is already overdue, preserves its error", func(t *testing.T) {
		backend, controller := newController(t)
		backend.lifetimes[power.UserActivity] = -time.Second
		backend.completeOnAcquire = errors.New("native assertion failed before deadline")
		start(t, controller, session.Request{PowerOptions: power.Options{Behaviors: power.UserActivity}})
		if snapshot := controller.Current(); snapshot.Status != session.Inactive || snapshot.Error == "" {
			t.Fatalf("current=%+v", snapshot)
		}
	})
	t.Run("when the platform exits unexpectedly, exposes a failure", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			backend, controller := newController(t)
			start(t, controller, idleRequest(0))
			backend.latestHold().complete(errors.New("native request failed"))
			synctest.Wait()
			if snapshot := controller.Current(); snapshot.Status != session.Inactive || snapshot.Error == "" {
				t.Fatalf("current=%+v", snapshot)
			}
		})
	})
}

func TestSessionShutdown(t *testing.T) {
	t.Run("when power release fails, still closes the PID watcher", func(t *testing.T) {
		backend := &controlledBackend{lifetimes: make(map[power.Behaviors]time.Duration), processWatch: newControlledHold(nil)}
		controller := session.NewController(backend)
		request := idleRequest(0)
		request.WatchedPID = 123
		start(t, controller, request)
		hold := backend.latestHold()
		hold.setReleaseError(errors.New("could not terminate native request"))
		defer func() { hold.setReleaseError(nil); _ = hold.Release() }()
		if err := controller.Close(); err == nil {
			t.Fatal("expected the release failure")
		}
		if !backend.processWatch.isReleased() || controller.Current().Status != session.Closed {
			t.Fatalf("watch released=%v current=%+v", backend.processWatch.isReleased(), controller.Current())
		}
	})
	t.Run("when an observer is idle, still releases the session", func(t *testing.T) {
		backend, controller := newController(t)
		for range 5 {
			start(t, controller, idleRequest(0))
		}
		for range 2 {
			if err := controller.Close(); err != nil {
				t.Fatal(err)
			}
		}
		if controller.Current().Status != session.Closed || backend.activeCount() != 0 {
			t.Fatalf("current=%+v active=%d", controller.Current(), backend.activeCount())
		}
		if _, err := controller.Start(context.Background(), idleRequest(0)); !errors.Is(err, session.ErrClosed) {
			t.Fatalf("start after close: %v", err)
		}
	})
	t.Run("when commands race with closing, releases every request", func(t *testing.T) {
		backend, controller := newController(t)
		var requests sync.WaitGroup
		for range 20 {
			requests.Go(func() { _, _ = controller.Start(context.Background(), idleRequest(0)) })
		}
		requests.Go(func() { _ = controller.Close() })
		requests.Wait()
		if backend.activeCount() != 0 || backend.maximumActive > 1 {
			t.Fatalf("active=%d peak=%d", backend.activeCount(), backend.maximumActive)
		}
	})
}

func idleRequest(duration time.Duration) session.Request {
	return session.Request{PowerOptions: power.Options{Behaviors: power.IdleSystemSleep, Duration: duration}}
}

func start(t *testing.T, controller *session.Controller, request session.Request) session.Snapshot {
	t.Helper()
	snapshot, err := controller.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func newController(t *testing.T) (*controlledBackend, *session.Controller) {
	t.Helper()
	backend := &controlledBackend{lifetimes: make(map[power.Behaviors]time.Duration), processWatch: newControlledHold(nil)}
	controller := session.NewController(backend)
	t.Cleanup(func() {
		if err := controller.Close(); err != nil {
			t.Errorf("close controller: %v", err)
		}
	})
	return backend, controller
}

type controlledBackend struct {
	mu                sync.Mutex
	holds             []*controlledHold
	lifetimes         map[power.Behaviors]time.Duration
	acquireErr        error
	processWatch      *controlledHold
	maximumActive     int
	completeOnAcquire error
	watchErr          error
}

func (backend *controlledBackend) Acquire(_ context.Context, options power.Options) (power.Hold, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.acquireErr != nil {
		return nil, backend.acquireErr
	}
	var assertions []power.Assertion
	for _, behavior := range []power.Behaviors{power.IdleSystemSleep, power.DisplaySleep, power.DiskIdleSleep, power.ACSystemSleep, power.UserActivity} {
		if options.Behaviors&behavior != 0 {
			assertion := power.Assertion{Behavior: behavior}
			if duration := backend.lifetimes[behavior]; duration != 0 {
				assertion.ExpiresAt = time.Now().Round(0).Add(duration)
			}
			assertions = append(assertions, assertion)
		}
	}
	hold := newControlledHold(assertions)
	if backend.completeOnAcquire != nil {
		hold.complete(backend.completeOnAcquire)
	}
	backend.holds = append(backend.holds, hold)
	active := 0
	for _, hold := range backend.holds {
		if !hold.isReleased() {
			active++
		}
	}
	backend.maximumActive = max(backend.maximumActive, active)
	return hold, nil
}

func (backend *controlledBackend) WatchProcess(int) (power.Watch, error) {
	return backend.processWatch, backend.watchErr
}

func (backend *controlledBackend) latestHold() *controlledHold {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return backend.holds[len(backend.holds)-1]
}

func (backend *controlledBackend) activeCount() int {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	count := 0
	for _, hold := range backend.holds {
		if !hold.isReleased() {
			count++
		}
	}
	return count
}

type controlledHold struct {
	mu         sync.Mutex
	assertions []power.Assertion
	done       chan struct{}
	doneOnce   sync.Once
	released   bool
	releaseErr error
	exitErr    error
}

func newControlledHold(assertions []power.Assertion) *controlledHold {
	return &controlledHold{assertions: assertions, done: make(chan struct{})}
}

func (hold *controlledHold) Assertions() []power.Assertion {
	return append([]power.Assertion(nil), hold.assertions...)
}
func (hold *controlledHold) Done() <-chan struct{} { return hold.done }
func (hold *controlledHold) Close() error          { return hold.Release() }

func (hold *controlledHold) Err() error {
	hold.mu.Lock()
	defer hold.mu.Unlock()
	return hold.exitErr
}

func (hold *controlledHold) Release() error {
	hold.mu.Lock()
	defer hold.mu.Unlock()
	if hold.releaseErr != nil {
		return hold.releaseErr
	}
	hold.released = true
	hold.doneOnce.Do(func() { close(hold.done) })
	return nil
}

func (hold *controlledHold) complete(err error) {
	hold.mu.Lock()
	defer hold.mu.Unlock()
	hold.exitErr = err
	hold.doneOnce.Do(func() { close(hold.done) })
}

func (hold *controlledHold) isReleased() bool {
	hold.mu.Lock()
	defer hold.mu.Unlock()
	return hold.released
}

func (hold *controlledHold) setReleaseError(err error) {
	hold.mu.Lock()
	defer hold.mu.Unlock()
	hold.releaseErr = err
}
