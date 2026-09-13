package integration_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestPackagedFailureCleanup(t *testing.T) {
	bundlePath := os.Getenv("KAFFEINATE_TEST_APP_BUNDLE")
	if bundlePath == "" {
		t.Skip("set KAFFEINATE_TEST_APP_BUNDLE to run packaged macOS checks")
	}
	if len(appPIDs()) != 0 {
		t.Skip("quit the existing Kaffeinate application before running packaged checks")
	}
	for _, testCase := range []struct{ name, mode string }{
		{"when a check fails after launch, cleans up the app", "launch"},
		{"when a check fails after restart, cleans up the app", "restart"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Cleanup(func() { cleanupOwnedApps(t, bundlePath) })
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			helper := exec.CommandContext(ctx, executable, "-test.run=^TestPackagedCleanupHelper$")
			helper.Env = append(os.Environ(), "KAFFEINATE_CLEANUP_TEST_CASE="+testCase.mode)
			output, err := helper.CombinedOutput()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
				t.Fatalf("expected deliberate test failure: %v: %s", err, output)
			}
			if !strings.Contains(string(output), "cleanup-checkpoint:"+testCase.mode+":") {
				t.Fatalf("helper failed before reaching the intended cleanup scenario: %s", output)
			}
			if pids := ownedAppPIDs(bundlePath); len(pids) != 0 {
				t.Fatalf("failed check leaked app instances: %v: %s", pids, output)
			}
		})
	}
}

func TestPackagedCleanupHelper(t *testing.T) {
	mode := os.Getenv("KAFFEINATE_CLEANUP_TEST_CASE")
	if mode == "" {
		return
	}
	bundlePath := os.Getenv("KAFFEINATE_TEST_APP_BUNDLE")
	t.Cleanup(func() { cleanupOwnedApps(t, bundlePath) })
	cliPath := filepath.Join(bundlePath, "Contents", "Resources", "bin", "kaffeinate")
	runCLI(t, cliPath, "/tmp", "-i", "-t28000")
	if mode == "restart" {
		pid := ownedAppPIDs(bundlePath)[0]
		_ = syscall.Kill(pid, syscall.SIGKILL)
		waitFor(t, func() bool { return !processRunning(pid) })
		runCLI(t, cliPath, "/tmp", "-i", "-t28000")
	}
	waitFor(t, func() bool { return len(ownedAppPIDs(bundlePath)) == 1 })
	checkpointPID := ownedAppPIDs(bundlePath)[0]
	waitFor(t, func() bool { return ownedAssertions(t, checkpointPID) != "" })
	fmt.Printf("cleanup-checkpoint:%s:%d\n", mode, checkpointPID)
	t.Fatal("deliberate failure before recording an acknowledged app PID")
}
