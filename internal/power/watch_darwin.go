package power

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"syscall"
)

func (Native) WatchProcess(pid int) (Watch, error) {
	if pid < 1 || pid > math.MaxInt32 {
		return nil, fmt.Errorf("invalid watched PID %d", pid)
	}
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	queueFD, err := syscall.Kqueue()
	if err != nil {
		return nil, fmt.Errorf("create process watch: %w", err)
	}
	syscall.CloseOnExec(queueFD)
	var wakePipe [2]int
	if err := syscall.Pipe(wakePipe[:]); err != nil {
		_ = syscall.Close(queueFD)
		return nil, err
	}
	syscall.CloseOnExec(wakePipe[0])
	syscall.CloseOnExec(wakePipe[1])
	watch := &processWatch{done: make(chan struct{}), wakeFD: wakePipe[1]}
	registrations := []syscall.Kevent_t{
		{Ident: uint64(pid), Filter: syscall.EVFILT_PROC, Flags: syscall.EV_ADD | syscall.EV_ONESHOT, Fflags: syscall.NOTE_EXIT},
		{Ident: uint64(wakePipe[0]), Filter: syscall.EVFILT_READ, Flags: syscall.EV_ADD | syscall.EV_ONESHOT},
	}
	if _, err := syscall.Kevent(queueFD, registrations, nil, nil); err != nil {
		_ = syscall.Close(queueFD)
		_ = syscall.Close(wakePipe[0])
		_ = syscall.Close(wakePipe[1])
		if errors.Is(err, syscall.ESRCH) {
			watch.closeOnce.Do(func() {})
			close(watch.done)
			return watch, nil
		}
		return nil, fmt.Errorf("watch PID %d: %w", pid, err)
	}
	go func() {
		defer close(watch.done)
		defer syscall.Close(queueFD)
		defer syscall.Close(wakePipe[0])
		watchEvents := make([]syscall.Kevent_t, 1)
		for {
			count, err := syscall.Kevent(queueFD, nil, watchEvents, nil)
			if errors.Is(err, syscall.EINTR) {
				continue
			}
			if err != nil {
				watch.watchErr = fmt.Errorf("observe PID %d: %w", pid, err)
			} else if count > 0 && watchEvents[0].Flags&syscall.EV_ERROR != 0 {
				watch.watchErr = fmt.Errorf("observe PID %d: %w", pid, syscall.Errno(watchEvents[0].Data))
			}
			watch.closeOnce.Do(func() { _ = syscall.Close(watch.wakeFD) })
			return
		}
	}()
	return watch, nil
}

type processWatch struct {
	done      chan struct{}
	wakeFD    int
	closeOnce sync.Once
	watchErr  error
}

func (watch *processWatch) Done() <-chan struct{} { return watch.done }

func (watch *processWatch) Err() error {
	select {
	case <-watch.done:
		return watch.watchErr
	default:
		return nil
	}
}

func (watch *processWatch) Close() error {
	watch.closeOnce.Do(func() { _ = syscall.Close(watch.wakeFD) })
	<-watch.done
	return watch.watchErr
}
