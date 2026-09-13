package control

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

func DefaultDirectory() string {
	return filepath.Join("/tmp", "kaffeinate-"+strconv.Itoa(os.Getuid()))
}

func SocketPath(directory string) string { return filepath.Join(directory, "control.sock") }

func awaitInstance(ctx context.Context, directory string) (*net.UnixListener, *os.File, error) {
	retryTicker := time.NewTicker(50 * time.Millisecond)
	defer retryTicker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		listener, lockFile, err := listenInstance(directory)
		if !errors.Is(err, ErrAlreadyRunning) {
			return listener, lockFile, err
		}
		connection, probeErr := (&net.Dialer{Timeout: 100 * time.Millisecond}).DialContext(ctx, "unix", SocketPath(directory))
		if probeErr == nil {
			_ = connection.Close()
			return nil, nil, ErrAlreadyRunning
		}
		if !errors.Is(probeErr, os.ErrNotExist) && !errors.Is(probeErr, syscall.ECONNREFUSED) {
			return nil, nil, fmt.Errorf("contact existing instance: %w", probeErr)
		}
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-retryTicker.C:
		}
	}
}

func listenInstance(directory string) (*net.UnixListener, *os.File, error) {
	if len(SocketPath(directory)) >= 104 {
		return nil, nil, errors.New("control socket path is too long")
	}
	if err := os.Mkdir(directory, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, nil, fmt.Errorf("create runtime directory: %w", err)
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, nil, err
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || info.Mode().Perm() != 0700 || !ok || owner.Uid != uint32(os.Getuid()) {
		return nil, nil, fmt.Errorf("runtime directory must be owned by you with mode 0700: %s", directory)
	}
	lockFile, err := os.OpenFile(filepath.Join(directory, "instance.lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, nil, fmt.Errorf("open instance lock: %w", err)
	}
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lockFile.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, nil, ErrAlreadyRunning
		}
		return nil, nil, fmt.Errorf("lock application instance: %w", err)
	}
	socketPath := SocketPath(directory)
	if socketInfo, err := os.Lstat(socketPath); err == nil {
		if socketInfo.Mode()&os.ModeSocket == 0 {
			_ = lockFile.Close()
			return nil, nil, fmt.Errorf("control path is not a socket: %s", socketPath)
		}
		if err := os.Remove(socketPath); err != nil {
			_ = lockFile.Close()
			return nil, nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		_ = lockFile.Close()
		return nil, nil, err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		_ = lockFile.Close()
		return nil, nil, fmt.Errorf("listen for CLI requests: %w", err)
	}
	if err := os.Chmod(socketPath, 0600); err != nil {
		_ = listener.Close()
		_ = lockFile.Close()
		return nil, nil, err
	}
	return listener, lockFile, nil
}
