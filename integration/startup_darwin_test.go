package integration_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestConcurrentPackagedLaunch(t *testing.T) {
	bundlePath := os.Getenv("KAFFEINATE_TEST_APP_BUNDLE")
	if bundlePath == "" {
		t.Skip("set KAFFEINATE_TEST_APP_BUNDLE to run packaged macOS checks")
	}
	if len(appPIDs()) != 0 {
		t.Skip("quit the existing Kaffeinate application before running packaged checks")
	}
	cliPath := filepath.Join(bundlePath, "Contents", "Resources", "bin", "kaffeinate")
	t.Cleanup(func() { cleanupOwnedApps(t, bundlePath) })
	runCLI(t, cliPath, "/tmp", "-i", "-t1")
	initialPID := appPIDs()[0]
	_ = syscall.Kill(initialPID, syscall.SIGTERM)
	waitFor(t, func() bool { return !processRunning(initialPID) })
	var clients sync.WaitGroup
	for range 6 {
		clients.Go(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if output, err := exec.CommandContext(ctx, cliPath, "-i", "-t30").CombinedOutput(); err != nil {
				t.Errorf("concurrent start: %v: %s", err, output)
			}
		})
	}
	clients.Wait()
	waitFor(t, func() bool { return len(appPIDs()) == 1 })
	if pids := appPIDs(); len(pids) != 1 {
		t.Fatalf("expected one owning app, got %v", pids)
	}
}
