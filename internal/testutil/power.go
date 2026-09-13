package testutil

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"kaffeinate/internal/power"
	"kaffeinate/internal/session"
)

type PowerBackend struct {
	ActiveRequests atomic.Int64
	AcquireError   error
	ReleaseStarted chan struct{}
	ReleaseGate    <-chan struct{}
}

func (backend *PowerBackend) Acquire(_ context.Context, options power.Options) (power.Hold, error) {
	if backend.AcquireError != nil {
		return nil, backend.AcquireError
	}
	backend.ActiveRequests.Add(1)
	return &powerHold{backend: backend, behaviors: options.Behaviors, done: make(chan struct{})}, nil
}

func (*PowerBackend) WatchProcess(int) (power.Watch, error) {
	return nil, errors.New("this fixture has no watched process")
}

type powerHold struct {
	backend   *PowerBackend
	behaviors power.Behaviors
	done      chan struct{}
	release   sync.Once
}

func (hold *powerHold) Assertions() []power.Assertion {
	return []power.Assertion{{Behavior: hold.behaviors}}
}

func (hold *powerHold) Done() <-chan struct{} { return hold.done }
func (hold *powerHold) Err() error            { return nil }
func (hold *powerHold) Release() error {
	hold.release.Do(func() {
		if hold.backend.ReleaseStarted != nil {
			close(hold.backend.ReleaseStarted)
		}
		if hold.backend.ReleaseGate != nil {
			<-hold.backend.ReleaseGate
		}
		hold.backend.ActiveRequests.Add(-1)
		close(hold.done)
	})
	return nil
}

func Controller(t *testing.T) (*session.Controller, *PowerBackend) {
	t.Helper()
	backend := &PowerBackend{}
	controller := session.NewController(backend)
	t.Cleanup(func() {
		if err := controller.Close(); err != nil {
			t.Error(err)
		}
	})
	return controller, backend
}

func RuntimeDirectory(t *testing.T) string {
	t.Helper()
	directory, err := os.MkdirTemp("/tmp", "kaffeinate-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	return directory
}
