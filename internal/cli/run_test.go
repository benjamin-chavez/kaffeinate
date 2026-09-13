package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"kaffeinate/internal/cli"
	"kaffeinate/internal/control"
	"kaffeinate/internal/power"
	"kaffeinate/internal/session"
	"kaffeinate/internal/testutil"
)

func TestCLIStart(t *testing.T) {
	t.Run("when the app is stopped, launches and starts a session", func(t *testing.T) {
		directory := testutil.RuntimeDirectory(t)
		controller, backend := testutil.Controller(t)
		runner := cli.Runner{
			Client: control.Client{SocketPath: control.SocketPath(directory)},
			Launch: func(context.Context) error { startControlServer(t, directory, controller); return nil },
		}
		var stdout, stderr bytes.Buffer
		if code := runner.Run(context.Background(), []string{"-i", "-t", "28000"}, &stdout, &stderr); code != 0 {
			t.Fatalf("exit=%d error=%s", code, &stderr)
		}
		if backend.ActiveRequests.Load() != 1 || !strings.Contains(stdout.String(), "You can close this terminal") || controller.Current().Status != session.Active {
			t.Fatalf("output=%s state=%+v", &stdout, controller.Current())
		}
	})
	t.Run("when the app is running, replaces without launching", func(t *testing.T) {
		directory := testutil.RuntimeDirectory(t)
		controller, backend := testutil.Controller(t)
		startControlServer(t, directory, controller)
		runner := cli.Runner{
			Client: control.Client{SocketPath: control.SocketPath(directory)},
			Launch: func(context.Context) error { t.Fatal("unexpected app launch"); return nil },
		}
		var stdout, stderr bytes.Buffer
		if code := runner.Run(context.Background(), []string{"-i"}, &stdout, &stderr); code != 0 {
			t.Fatalf("exit=%d error=%s", code, &stderr)
		}
		if code := runner.Run(context.Background(), []string{"-d", "-t1"}, &stdout, &stderr); code != 0 {
			t.Fatalf("exit=%d error=%s", code, &stderr)
		}
		if controller.Current().Behaviors != power.DisplaySleep || backend.ActiveRequests.Load() != 1 {
			t.Fatalf("current=%+v active=%d", controller.Current(), backend.ActiveRequests.Load())
		}
	})
}

func TestCLISideEffects(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		arguments []string
		exitCode  int
	}{
		{"when help is requested, preserves the active session", []string{"-h"}, 0},
		{"when arguments are invalid, preserves the active session", []string{"-t-5"}, 2},
		{"when a command is wrapped, rejects it without changes", []string{"make"}, 2},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			directory := testutil.RuntimeDirectory(t)
			controller, _ := testutil.Controller(t)
			before, err := controller.Start(context.Background(), session.Request{PowerOptions: power.Options{Behaviors: power.IdleSystemSleep}})
			if err != nil {
				t.Fatal(err)
			}
			runner := cli.Runner{
				Client: control.Client{SocketPath: control.SocketPath(directory)},
				Launch: func(context.Context) error { t.Fatal("unexpected launch"); return nil },
			}
			var stdout, stderr bytes.Buffer
			if code := runner.Run(context.Background(), testCase.arguments, &stdout, &stderr); code != testCase.exitCode || controller.Current() != before {
				t.Fatalf("exit=%d state=%+v stdout=%s stderr=%s", code, controller.Current(), &stdout, &stderr)
			}
			if testCase.exitCode == 0 && !strings.Contains(stdout.String(), "Usage: kaffeinate") {
				t.Fatalf("help output=%s", &stdout)
			}
		})
	}
}

func TestCLIFailure(t *testing.T) {
	t.Run("when launch fails, explains the operational failure", func(t *testing.T) {
		runner := cli.Runner{
			Client: control.Client{SocketPath: control.SocketPath(testutil.RuntimeDirectory(t))},
			Launch: func(context.Context) error { return errors.New("Launch Services unavailable") },
		}
		var stdout, stderr bytes.Buffer
		if code := runner.Run(context.Background(), nil, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "Launch Services unavailable") {
			t.Fatalf("exit=%d error=%s", code, &stderr)
		}
	})
	t.Run("when readiness times out, exits with an error", func(t *testing.T) {
		runner := cli.Runner{
			Client: control.Client{SocketPath: control.SocketPath(testutil.RuntimeDirectory(t))},
			Launch: func(context.Context) error { return nil },
		}
		ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
		defer cancel()
		var stdout, stderr bytes.Buffer
		if code := runner.Run(ctx, nil, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "deadline exceeded") {
			t.Fatalf("exit=%d error=%s", code, &stderr)
		}
	})
	t.Run("when acknowledgement is lost, leaves the accepted session running", func(t *testing.T) {
		directory := testutil.RuntimeDirectory(t)
		controller, backend := testutil.Controller(t)
		listener, err := net.Listen("unix", control.SocketPath(directory))
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
		handled := make(chan struct{})
		acceptedRequests := 0
		var firstDeadline time.Time
		go func() {
			defer close(handled)
			for {
				connection, err := listener.Accept()
				if errors.Is(err, net.ErrClosed) {
					return
				}
				if err != nil {
					t.Error(err)
					return
				}
				_ = connection.SetDeadline(time.Now().Add(time.Second))
				var request control.Request
				if err := json.NewDecoder(connection).Decode(&request); err != nil {
					_ = connection.Close()
					t.Error(err)
					return
				}
				snapshot, err := controller.Start(context.Background(), request.Session)
				if err != nil {
					t.Error(err)
				}
				acceptedRequests++
				if acceptedRequests == 1 {
					firstDeadline = snapshot.Deadline
				}
				_ = connection.Close()
			}
		}()
		runner := cli.Runner{
			Client: control.Client{SocketPath: control.SocketPath(directory)},
			Launch: func(context.Context) error { t.Fatal("unexpected retry or launch"); return nil },
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		var stdout, stderr bytes.Buffer
		if code := runner.Run(ctx, []string{"-t30"}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "could not confirm") {
			t.Fatalf("exit=%d error=%s", code, &stderr)
		}
		_ = listener.Close()
		<-handled
		if backend.ActiveRequests.Load() != 1 || controller.Current().Status != session.Active || acceptedRequests != 1 || controller.Current().Deadline != firstDeadline {
			t.Fatalf("accepted requests=%d session=%+v", acceptedRequests, controller.Current())
		}
	})
}

func TestCLIAcknowledgement(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		state any
	}{
		{"when state is missing, reports an uncertain outcome", nil},
		{"when state is empty, reports an uncertain outcome", map[string]any{}},
		{"when status is unknown, reports an uncertain outcome", map[string]any{"status": "unknown"}},
		{"when active behavior is missing, reports an uncertain outcome", map[string]any{"status": "active"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			directory := testutil.RuntimeDirectory(t)
			listener, err := net.Listen("unix", control.SocketPath(directory))
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			handled := make(chan struct{})
			go func() {
				defer close(handled)
				connection, err := listener.Accept()
				if err != nil {
					t.Error(err)
					return
				}
				defer connection.Close()
				var request control.Request
				if err := json.NewDecoder(connection).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				response := map[string]any{"version": control.ProtocolVersion, "id": request.ID}
				if testCase.state != nil {
					response["state"] = testCase.state
				}
				_ = json.NewEncoder(connection).Encode(response)
			}()
			runner := cli.Runner{
				Client: control.Client{SocketPath: control.SocketPath(directory)},
				Launch: func(context.Context) error { t.Fatal("unexpected launch"); return nil },
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			var stdout, stderr bytes.Buffer
			if code := runner.Run(ctx, nil, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "could not confirm") || stdout.Len() != 0 {
				t.Fatalf("exit=%d output=%s error=%s", code, &stdout, &stderr)
			}
			<-handled
		})
	}
}

func startControlServer(t *testing.T, directory string, controller *session.Controller) {
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
}
