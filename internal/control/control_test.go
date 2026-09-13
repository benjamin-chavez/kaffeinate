package control_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"kaffeinate/internal/control"
	"kaffeinate/internal/power"
	"kaffeinate/internal/session"
	"kaffeinate/internal/testutil"
)

func TestControlStart(t *testing.T) {
	t.Run("when a request is acknowledged, owns an active session", func(t *testing.T) {
		directory := testutil.RuntimeDirectory(t)
		controller, backend := testutil.Controller(t)
		listen(t, directory, controller)
		client := control.Client{SocketPath: control.SocketPath(directory)}
		beforeSnapshot, err := client.Start(context.Background(), request(time.Hour))
		if err != nil || beforeSnapshot.Status != session.Active || backend.ActiveRequests.Load() != 1 {
			t.Fatalf("response=%+v error=%v", beforeSnapshot, err)
		}
		afterSnapshot, err := client.Start(context.Background(), request(2*time.Hour))
		if err != nil || !afterSnapshot.Deadline.After(beforeSnapshot.Deadline) || backend.ActiveRequests.Load() != 1 {
			t.Fatalf("replacement=%+v error=%v active=%d", afterSnapshot, err, backend.ActiveRequests.Load())
		}
	})
	t.Run("when activation fails, returns the application error", func(t *testing.T) {
		directory := testutil.RuntimeDirectory(t)
		controller, backend := testutil.Controller(t)
		backend.AcquireError = errors.New("power access failed")
		listen(t, directory, controller)
		_, err := (control.Client{SocketPath: control.SocketPath(directory)}).Start(context.Background(), request(0))
		if err == nil || !strings.Contains(err.Error(), "power access failed") || backend.ActiveRequests.Load() != 0 {
			t.Fatalf("error=%v active=%d", err, backend.ActiveRequests.Load())
		}
	})
	t.Run("when clients arrive together, retains one active session", func(t *testing.T) {
		directory := testutil.RuntimeDirectory(t)
		controller, backend := testutil.Controller(t)
		listen(t, directory, controller)
		var clients sync.WaitGroup
		for range 20 {
			clients.Go(func() {
				_, err := (control.Client{SocketPath: control.SocketPath(directory)}).Start(context.Background(), request(0))
				if err != nil {
					t.Error(err)
				}
			})
		}
		clients.Wait()
		if backend.ActiveRequests.Load() != 1 {
			t.Fatalf("active=%d", backend.ActiveRequests.Load())
		}
	})
}

func TestControlValidation(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		message string
	}{
		{"when JSON is malformed, preserves the session", "{invalid}\n"},
		{"when the message is incomplete, preserves the session", `{"version":1`},
		{"when the protocol differs, preserves the session", `{"version":2,"id":"test","session":{"power":{"behaviors":1,"duration":0}}}` + "\n"},
		{"when behavior is unknown, preserves the session", `{"version":1,"id":"test","session":{"power":{"behaviors":255,"duration":0}}}` + "\n"},
		{"when the message is oversized, preserves the session", strings.Repeat("x", control.MaxMessageBytes+1) + "\n"},
		{"when trailing JSON is present, preserves the session", "{} {}\n"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			directory := testutil.RuntimeDirectory(t)
			controller, backend := testutil.Controller(t)
			listen(t, directory, controller)
			beforeSnapshot, err := controller.Start(context.Background(), request(time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			connection, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: control.SocketPath(directory), Net: "unix"})
			if err != nil {
				t.Fatal(err)
			}
			defer connection.Close()
			_ = connection.SetDeadline(time.Now().Add(time.Second))
			_, _ = connection.Write([]byte(testCase.message))
			_ = connection.CloseWrite()
			var response control.Response
			if err := json.NewDecoder(connection).Decode(&response); err != nil || response.Error == "" {
				t.Fatalf("response=%+v error=%v", response, err)
			}
			if afterSnapshot := controller.Current(); beforeSnapshot != afterSnapshot || backend.ActiveRequests.Load() != 1 {
				t.Fatalf("before=%+v after=%+v", beforeSnapshot, afterSnapshot)
			}
		})
	}
}

func TestApplicationInstance(t *testing.T) {
	t.Run("when a second instance starts, preserves the first socket", func(t *testing.T) {
		directory := testutil.RuntimeDirectory(t)
		controller, _ := testutil.Controller(t)
		listen(t, directory, controller)
		if _, err := control.Listen(context.Background(), directory, controller); !errors.Is(err, control.ErrAlreadyRunning) {
			t.Fatalf("second instance: %v", err)
		}
		if _, err := (control.Client{SocketPath: control.SocketPath(directory)}).Start(context.Background(), request(0)); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(control.SocketPath(directory))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("socket info=%v error=%v", info, err)
		}
	})
	t.Run("when a socket is stale, starts a new instance", func(t *testing.T) {
		directory := testutil.RuntimeDirectory(t)
		staleListener, err := net.ListenUnix("unix", &net.UnixAddr{Name: control.SocketPath(directory), Net: "unix"})
		if err != nil {
			t.Fatal(err)
		}
		staleListener.SetUnlinkOnClose(false)
		_ = staleListener.Close()
		controller, _ := testutil.Controller(t)
		listen(t, directory, controller)
		if _, err := (control.Client{SocketPath: control.SocketPath(directory)}).Start(context.Background(), request(0)); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("when an instance crashes, releases its ownership", func(t *testing.T) {
		directory := testutil.RuntimeDirectory(t)
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		helper := exec.CommandContext(ctx, executable, "-test.run=^TestInstanceHelper$")
		helper.Env = append(os.Environ(), "KAFFEINATE_INSTANCE_TEST_DIRECTORY="+directory)
		stdout, err := helper.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := helper.Start(); err != nil {
			t.Fatal(err)
		}
		defer helper.Process.Kill()
		if !bufio.NewScanner(stdout).Scan() {
			_ = helper.Wait()
			t.Fatal("helper did not become ready")
		}
		_ = helper.Process.Kill()
		_ = helper.Wait()
		controller, _ := testutil.Controller(t)
		listen(t, directory, controller)
		if _, err := (control.Client{SocketPath: control.SocketPath(directory)}).Start(context.Background(), request(0)); err != nil {
			t.Fatal(err)
		}
	})
}

func TestControlShutdown(t *testing.T) {
	t.Run("while power cleanup is pending, retains instance ownership", func(t *testing.T) {
		directory := testutil.RuntimeDirectory(t)
		controller, backend := testutil.Controller(t)
		server := listen(t, directory, controller)
		releaseGate := make(chan struct{})
		backend.ReleaseStarted = make(chan struct{})
		backend.ReleaseGate = releaseGate
		var unblockOnce sync.Once
		unblock := func() { unblockOnce.Do(func() { close(releaseGate) }) }
		defer unblock()
		if _, err := controller.Start(context.Background(), request(0)); err != nil {
			t.Fatal(err)
		}
		shutdownDone := make(chan error, 1)
		go func() {
			requestErr := server.StopRequests()
			sessionErr := controller.Close()
			shutdownDone <- errors.Join(requestErr, sessionErr, server.Close())
		}()
		<-backend.ReleaseStarted
		ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
		defer cancel()
		if competing, err := control.Listen(ctx, directory, controller); !errors.Is(err, context.DeadlineExceeded) {
			if competing != nil {
				_ = competing.Close()
			}
			t.Fatalf("competing start=%v", err)
		}
		unblock()
		if err := <-shutdownDone; err != nil {
			t.Fatal(err)
		}
		listen(t, directory, controller)
	})
	t.Run("when a client stalls, closing still releases the socket", func(t *testing.T) {
		directory := testutil.RuntimeDirectory(t)
		controller, _ := testutil.Controller(t)
		server := listen(t, directory, controller)
		connection, err := net.Dial("unix", control.SocketPath(directory))
		if err != nil {
			t.Fatal(err)
		}
		defer connection.Close()
		_, _ = connection.Write([]byte("{"))
		if err := server.Close(); err != nil {
			t.Fatal(err)
		}
		listen(t, directory, controller)
	})
}

func TestInstanceHelper(t *testing.T) {
	directory := os.Getenv("KAFFEINATE_INSTANCE_TEST_DIRECTORY")
	if directory == "" {
		return
	}
	controller := session.NewController(&testutil.PowerBackend{})
	server, err := control.Listen(context.Background(), directory, controller)
	if err != nil {
		os.Exit(2)
	}
	defer server.Close()
	defer controller.Close()
	fmt.Println("ready")
	time.Sleep(time.Hour)
	os.Exit(0)
}

func listen(t *testing.T, directory string, controller *session.Controller) *control.Server {
	t.Helper()
	server, err := control.Listen(context.Background(), directory, controller)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := server.Close(); err != nil {
			t.Error(err)
		}
	})
	return server
}

func request(duration time.Duration) session.Request {
	return session.Request{PowerOptions: power.Options{Behaviors: power.IdleSystemSleep, Duration: duration}}
}
