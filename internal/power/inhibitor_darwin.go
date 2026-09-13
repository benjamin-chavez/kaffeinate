package power

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Native struct{}

func (Native) Acquire(ctx context.Context, options Options) (Hold, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	arguments := []string{"-w", strconv.Itoa(os.Getpid())}
	for _, assertionFlag := range []struct {
		behavior Behaviors
		flag     string
	}{
		{IdleSystemSleep, "-i"}, {DisplaySleep, "-d"}, {DiskIdleSleep, "-m"},
		{ACSystemSleep, "-s"}, {UserActivity, "-u"},
	} {
		if options.Behaviors&assertionFlag.behavior != 0 {
			arguments = append(arguments, assertionFlag.flag)
		}
	}
	if options.Duration > 0 {
		arguments = append(arguments, "-t", strconv.FormatInt(int64(options.Duration/time.Second), 10))
	}
	processHold := &nativeHold{
		command: exec.Command("/usr/bin/caffeinate", arguments...),
		done:    make(chan struct{}),
	}
	processHold.command.Stderr = &processHold.stderr
	startedAt := time.Now().Round(0)
	if err := processHold.command.Start(); err != nil {
		return nil, fmt.Errorf("start caffeinate: %w", err)
	}
	processHold.assertions = nativeAssertions(options, startedAt)
	go func() {
		processHold.waitErr = processHold.command.Wait()
		close(processHold.done)
	}()
	return processHold, nil
}

func nativeAssertions(options Options, startedAt time.Time) []Assertion {
	var assertions []Assertion
	for _, behavior := range []Behaviors{IdleSystemSleep, DisplaySleep, DiskIdleSleep, ACSystemSleep, UserActivity} {
		if options.Behaviors&behavior == 0 {
			continue
		}
		assertion := Assertion{Behavior: behavior}
		duration := options.Duration
		// The mixed-assertion timeout is documented in docs/plans/02-session-requests-and-cli-parsing.md.
		if duration == 0 && options.Behaviors&UserActivity != 0 && behavior&(UserActivity|DiskIdleSleep) != 0 {
			duration = 5 * time.Second
		}
		if duration > 0 {
			assertion.ExpiresAt = startedAt.Add(duration)
		}
		assertions = append(assertions, assertion)
	}
	return assertions
}

type nativeHold struct {
	command    *exec.Cmd
	assertions []Assertion
	stderr     bytes.Buffer
	done       chan struct{}
	waitErr    error
	releaseMu  sync.Mutex
}

func (processHold *nativeHold) Assertions() []Assertion { return slices.Clone(processHold.assertions) }
func (processHold *nativeHold) Done() <-chan struct{}   { return processHold.done }

func (processHold *nativeHold) Err() error {
	select {
	case <-processHold.done:
		if processHold.waitErr != nil {
			return fmt.Errorf("caffeinate exited: %w %s", processHold.waitErr, strings.TrimSpace(processHold.stderr.String()))
		}
	default:
	}
	return nil
}

func (processHold *nativeHold) Release() error {
	processHold.releaseMu.Lock()
	defer processHold.releaseMu.Unlock()
	select {
	case <-processHold.done:
		return nil
	default:
	}
	_ = processHold.command.Process.Signal(syscall.SIGTERM)
	graceTimer := time.NewTimer(500 * time.Millisecond)
	defer graceTimer.Stop()
	select {
	case <-processHold.done:
		return nil
	case <-graceTimer.C:
	}
	if err := processHold.command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("stop caffeinate: %w", err)
	}
	killTimer := time.NewTimer(2 * time.Second)
	defer killTimer.Stop()
	select {
	case <-processHold.done:
		return nil
	case <-killTimer.C:
		return errors.New("caffeinate did not exit after termination")
	}
}
