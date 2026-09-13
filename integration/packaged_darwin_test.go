package integration_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"kaffeinate/internal/control"
)

func TestPackagedCLI(t *testing.T) {
	bundlePath := os.Getenv("KAFFEINATE_TEST_APP_BUNDLE")
	if bundlePath == "" {
		t.Skip("set KAFFEINATE_TEST_APP_BUNDLE to run packaged macOS checks")
	}
	if len(appPIDs()) != 0 {
		t.Skip("quit the existing Kaffeinate application before running packaged checks")
	}
	bundlePath, err := filepath.Abs(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	installationDirectory := t.TempDir()
	cliPath := filepath.Join(installationDirectory, "kaffeinate")
	if err := os.Symlink(filepath.Join(bundlePath, "Contents", "Resources", "bin", "kaffeinate"), cliPath); err != nil {
		t.Fatal(err)
	}
	ownedAppPID := 0
	t.Cleanup(func() { cleanupOwnedApps(t, bundlePath) })

	t.Run("when the terminal closes, keeps its background session", func(t *testing.T) {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		terminal := exec.CommandContext(ctx, executable, "-test.run=^TestTerminalHelper$")
		terminal.Env = append(os.Environ(), "KAFFEINATE_TERMINAL_TEST_CLI="+cliPath)
		terminal.Dir = installationDirectory
		terminal.Stderr = os.Stderr
		stdout, err := terminal.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := terminal.Start(); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = terminal.Process.Kill(); _ = terminal.Wait() }()
		output := bufio.NewScanner(stdout)
		if !output.Scan() || !strings.Contains(output.Text(), "You can close this terminal") {
			t.Fatal("CLI did not acknowledge its session")
		}
		waitFor(t, func() bool { return len(appPIDs()) == 1 })
		ownedAppPID = appPIDs()[0]
		if err := terminal.Process.Kill(); err != nil {
			t.Fatal(err)
		}
		waitFor(t, func() bool { return strings.Contains(ownedAssertions(t, ownedAppPID), "PreventUserIdleSystemSleep") })
		if !processRunning(ownedAppPID) {
			t.Fatal("the app exited with the terminal")
		}
	})
	if ownedAppPID == 0 {
		t.Fatal("the initial app was not started")
	}

	t.Run("when reopened or given invalid input, preserves its session", func(t *testing.T) {
		beforePIDs := nativePIDs(ownedAppPID)
		if output, err := exec.Command("/usr/bin/open", "-g", bundlePath).CombinedOutput(); err != nil {
			t.Fatalf("open: %v: %s", err, output)
		}
		command := exec.Command(cliPath, "-t-1")
		command.Dir = installationDirectory
		output, err := command.CombinedOutput()
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 {
			t.Fatalf("invalid input: %v: %s", err, output)
		}
		if strings.Join(nativePIDs(ownedAppPID), ",") != strings.Join(beforePIDs, ",") {
			t.Fatal("existing native session changed")
		}
	})

	t.Run("when replaced with a short timer, expires in the same app", func(t *testing.T) {
		runCLI(t, cliPath, installationDirectory, "-di", "-t2")
		waitFor(t, func() bool { return strings.Contains(ownedAssertions(t, ownedAppPID), "PreventUserIdleDisplaySleep") })
		waitFor(t, func() bool { return ownedAssertions(t, ownedAppPID) == "" && len(nativePIDs(ownedAppPID)) == 0 })
		if pids := appPIDs(); len(pids) != 1 || pids[0] != ownedAppPID {
			t.Fatalf("app instances=%v", pids)
		}
	})

	t.Run("when default activity expires, retains idle sleep prevention", func(t *testing.T) {
		runCLI(t, cliPath, installationDirectory, "-iu")
		waitFor(t, func() bool { return strings.Contains(ownedAssertions(t, ownedAppPID), "UserIsActive") })
		waitFor(t, func() bool {
			assertions := ownedAssertions(t, ownedAppPID)
			return !strings.Contains(assertions, "UserIsActive") && strings.Contains(assertions, "PreventUserIdleSystemSleep")
		})
	})

	for _, testCase := range []struct {
		name      string
		arguments []string
	}{
		{"when activity alone expires, ends the default session", []string{"-u"}},
		{"when activity is explicitly timed, ends at its timeout", []string{"-u", "-t1"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			runCLI(t, cliPath, installationDirectory, testCase.arguments...)
			waitFor(t, func() bool { return strings.Contains(ownedAssertions(t, ownedAppPID), "UserIsActive") })
			waitFor(t, func() bool { return ownedAssertions(t, ownedAppPID) == "" && len(nativePIDs(ownedAppPID)) == 0 })
		})
	}

	t.Run("when a watched process exits first, releases the session", func(t *testing.T) {
		watchedProcess := startSleeper(t)
		runCLI(t, cliPath, installationDirectory, "-i", "-t30", "-w", strconv.Itoa(watchedProcess.Process.Pid))
		waitFor(t, func() bool { return ownedAssertions(t, ownedAppPID) != "" })
		_ = watchedProcess.Process.Kill()
		waitFor(t, func() bool { return ownedAssertions(t, ownedAppPID) == "" && len(nativePIDs(ownedAppPID)) == 0 })
	})

	t.Run("when timeout arrives first, leaves the watched process running", func(t *testing.T) {
		watchedProcess := startSleeper(t)
		runCLI(t, cliPath, installationDirectory, "-i", "-t1", "-w", strconv.Itoa(watchedProcess.Process.Pid))
		waitFor(t, func() bool { return ownedAssertions(t, ownedAppPID) == "" && len(nativePIDs(ownedAppPID)) == 0 })
		if !processRunning(watchedProcess.Process.Pid) {
			t.Fatal("watched process was stopped")
		}
	})

	t.Run("when launched during cleanup, waits for the previous owner", func(t *testing.T) {
		runCLI(t, cliPath, installationDirectory, "-i")
		waitFor(t, func() bool { return len(nativePIDs(ownedAppPID)) == 1 })
		nativePID, err := strconv.Atoi(nativePIDs(ownedAppPID)[0])
		if err != nil {
			t.Fatal(err)
		}
		previousPID := ownedAppPID
		t.Cleanup(func() { cleanupNativeProcess(t, nativePID, previousPID) })
		if err := syscall.Kill(nativePID, syscall.SIGSTOP); err != nil {
			t.Fatal(err)
		}
		if err := syscall.Kill(previousPID, syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		waitFor(t, func() bool {
			_, err := os.Stat(control.SocketPath(control.DefaultDirectory()))
			return errors.Is(err, os.ErrNotExist)
		})
		runCLI(t, cliPath, installationDirectory, "-i", "-t1")
		waitFor(t, func() bool { return !processRunning(previousPID) && len(appPIDs()) == 1 })
		ownedAppPID = appPIDs()[0]
		waitFor(t, func() bool { return ownedAssertions(t, ownedAppPID) == "" && len(nativePIDs(ownedAppPID)) == 0 })
	})

	t.Run("when the app crashes, releases power and can restart", func(t *testing.T) {
		runCLI(t, cliPath, installationDirectory, "-i")
		waitFor(t, func() bool { return ownedAssertions(t, ownedAppPID) != "" })
		previousPID := ownedAppPID
		if err := syscall.Kill(previousPID, syscall.SIGKILL); err != nil {
			t.Fatal(err)
		}
		waitFor(t, func() bool { return !processRunning(previousPID) && ownedAssertions(t, previousPID) == "" })
		ownedAppPID = 0
		runCLI(t, cliPath, installationDirectory, "-i", "-t1")
		waitFor(t, func() bool { return len(appPIDs()) == 1 })
		ownedAppPID = appPIDs()[0]
		waitFor(t, func() bool { return ownedAssertions(t, ownedAppPID) == "" && len(nativePIDs(ownedAppPID)) == 0 })
	})

	t.Run("when clients launch together, creates one owning app", func(t *testing.T) {
		previousPID := ownedAppPID
		if err := syscall.Kill(previousPID, syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		waitFor(t, func() bool { return !processRunning(previousPID) })
		ownedAppPID = 0
		var clients sync.WaitGroup
		for range 6 {
			clients.Go(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				command := exec.CommandContext(ctx, cliPath, "-i", "-t30")
				command.Dir = installationDirectory
				if output, err := command.CombinedOutput(); err != nil {
					t.Errorf("CLI launch: %v: %s", err, output)
				}
			})
		}
		clients.Wait()
		waitFor(t, func() bool { return len(appPIDs()) == 1 })
		ownedAppPID = appPIDs()[0]
		waitFor(t, func() bool { return len(nativePIDs(ownedAppPID)) == 1 })
	})
}

func TestTerminalHelper(t *testing.T) {
	cliPath := os.Getenv("KAFFEINATE_TERMINAL_TEST_CLI")
	if cliPath == "" {
		return
	}
	output, err := exec.Command(cliPath, "-i", "-t28000").CombinedOutput()
	if err != nil {
		fmt.Fprintln(os.Stderr, string(output), err)
		os.Exit(2)
	}
	fmt.Print(string(output))
	time.Sleep(time.Hour)
	os.Exit(0)
}

func runCLI(t *testing.T, cliPath, workingDirectory string, arguments ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, cliPath, arguments...)
	command.Dir = workingDirectory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("CLI: %v: %s", err, output)
	}
}

func startSleeper(t *testing.T) *exec.Cmd {
	t.Helper()
	command := exec.Command("/bin/sleep", "30")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait() })
	return command
}

func processRunning(pid int) bool { return syscall.Kill(pid, 0) == nil }

func appPIDs() []int {
	output, _ := exec.Command("/usr/bin/pgrep", "-x", "Kaffeinate").Output()
	var pids []int
	for _, value := range strings.Fields(string(output)) {
		if pid, err := strconv.Atoi(value); err == nil {
			pids = append(pids, pid)
		}
	}
	return pids
}

func nativePIDs(ownerPID int) []string {
	output, _ := exec.Command("/usr/bin/pgrep", "-P", strconv.Itoa(ownerPID), "-x", "caffeinate").Output()
	return strings.Fields(string(output))
}

func ownedAssertions(t *testing.T, ownerPID int) string {
	t.Helper()
	output, err := exec.Command("/usr/bin/pmset", "-g", "assertions").Output()
	if err != nil {
		t.Fatal(err)
	}
	var header string
	var assertions []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "pid ") {
			header = line
		}
		if line == fmt.Sprintf("Created for PID: %d.", ownerPID) && strings.Contains(header, "(caffeinate)") {
			assertions = append(assertions, header)
		}
	}
	return strings.Join(assertions, "\n")
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if condition() {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("condition was not met before the deadline")
		case <-ticker.C:
		}
	}
}

func ownedAppPIDs(bundlePath string) []int {
	appExecutable, err := filepath.EvalSymlinks(filepath.Join(bundlePath, "Contents", "MacOS", "Kaffeinate"))
	if err != nil {
		return nil
	}
	var ownedPIDs []int
	for _, pid := range appPIDs() {
		output, err := exec.Command("/bin/ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
		if err != nil {
			continue
		}
		executable, err := filepath.EvalSymlinks(strings.TrimSpace(string(output)))
		if err == nil && executable == appExecutable {
			ownedPIDs = append(ownedPIDs, pid)
		}
	}
	return ownedPIDs
}

func cleanupOwnedApps(t *testing.T, bundlePath string) {
	t.Helper()
	for _, pid := range ownedAppPIDs(bundlePath) {
		_ = syscall.Kill(pid, syscall.SIGTERM)
		waitFor(t, func() bool { return !processRunning(pid) && ownedAssertions(t, pid) == "" })
	}
}

func cleanupNativeProcess(t *testing.T, nativePID, ownerPID int) {
	t.Helper()
	output, err := exec.Command("/bin/ps", "-p", strconv.Itoa(nativePID), "-o", "args=").Output()
	if err != nil {
		return
	}
	arguments := strings.Fields(string(output))
	if len(arguments) < 3 || arguments[0] != "/usr/bin/caffeinate" || arguments[1] != "-w" || arguments[2] != strconv.Itoa(ownerPID) {
		return
	}
	_ = syscall.Kill(nativePID, syscall.SIGCONT)
	_ = syscall.Kill(nativePID, syscall.SIGTERM)
	waitFor(t, func() bool { return !processRunning(nativePID) })
}
