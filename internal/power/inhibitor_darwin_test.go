package power_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"kaffeinate/internal/power"
)

func TestNativeSleepPrevention(t *testing.T) {
	t.Run("when native launch is unavailable, reports failure", func(t *testing.T) {
		startPowerHelper(t, "startfailure", power.Options{})
	})
	t.Run("when activity has an explicit timeout, outlives five seconds", func(t *testing.T) {
		hold := acquire(t, power.Options{Behaviors: power.UserActivity, Duration: 7 * time.Second})
		eventually(t, func() bool { return strings.Contains(assertionsFor(t, os.Getpid()), "UserIsActive") })
		checkpoint := time.NewTimer(5500 * time.Millisecond)
		defer checkpoint.Stop()
		select {
		case <-hold.Done():
			t.Fatal("activity ended before its explicit timeout")
		case <-checkpoint.C:
		}
		if assertions := assertionsFor(t, os.Getpid()); !strings.Contains(assertions, "UserIsActive") {
			t.Fatalf("activity assertion expired at its default timeout: %s", assertions)
		}
		await(t, hold.Done())
		if err := hold.Err(); err != nil {
			t.Fatal(err)
		}
		eventually(t, func() bool { return assertionsFor(t, os.Getpid()) == "" })
	})
	t.Run("when idle prevention is requested, allows display sleep", func(t *testing.T) {
		hold := acquire(t, power.Options{Behaviors: power.IdleSystemSleep})
		eventually(t, func() bool { return strings.Contains(assertionsFor(t, os.Getpid()), "PreventUserIdleSystemSleep") })
		if assertions := assertionsFor(t, os.Getpid()); strings.Contains(assertions, "PreventUserIdleDisplaySleep") {
			t.Fatalf("unexpected display assertion: %s", assertions)
		}
		for range 2 {
			if err := hold.Release(); err != nil {
				t.Fatal(err)
			}
		}
		await(t, hold.Done())
		eventually(t, func() bool { return assertionsFor(t, os.Getpid()) == "" })
	})
	t.Run("when display and system are requested, holds both assertions", func(t *testing.T) {
		hold := acquire(t, power.Options{Behaviors: power.IdleSystemSleep | power.DisplaySleep, Duration: 2 * time.Second})
		eventually(t, func() bool {
			assertions := assertionsFor(t, os.Getpid())
			return strings.Contains(assertions, "PreventUserIdleSystemSleep") && strings.Contains(assertions, "PreventUserIdleDisplaySleep")
		})
		await(t, hold.Done())
		if err := hold.Err(); err != nil {
			t.Fatal(err)
		}
		eventually(t, func() bool { return assertionsFor(t, os.Getpid()) == "" })
	})
	t.Run("when mixed activity expires, releases both native assertions", func(t *testing.T) {
		hold := acquire(t, power.Options{Behaviors: power.UserActivity | power.DiskIdleSleep})
		eventually(t, func() bool {
			assertions := assertionsFor(t, os.Getpid())
			return strings.Contains(assertions, "UserIsActive") && strings.Contains(assertions, "PreventDiskIdle")
		})
		for _, assertion := range hold.Assertions() {
			remaining := time.Until(assertion.ExpiresAt)
			if remaining <= 0 || remaining > 5*time.Second {
				t.Fatalf("unexpected assertion deadline: %+v", assertion)
			}
		}
		eventually(t, func() bool { return assertionsFor(t, os.Getpid()) == "" })
		select {
		case <-hold.Done():
			t.Fatal("native process should outlive its default assertions")
		default:
		}
	})
	t.Run("when the native child is killed, reports its failure", func(t *testing.T) {
		hold := acquire(t, power.Options{Behaviors: power.IdleSystemSleep})
		eventually(t, func() bool { return assertionsFor(t, os.Getpid()) != "" })
		pidOutput, err := exec.Command("/usr/bin/pgrep", "-P", strconv.Itoa(os.Getpid()), "-x", "caffeinate").Output()
		if err != nil {
			t.Fatal(err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(pidOutput)))
		if err != nil {
			t.Fatalf("owned native PID: %q: %v", pidOutput, err)
		}
		process, err := os.FindProcess(pid)
		if err != nil {
			t.Fatal(err)
		}
		if err := process.Kill(); err != nil {
			t.Fatal(err)
		}
		await(t, hold.Done())
		if hold.Err() == nil {
			t.Fatal("expected an observable child-exit failure")
		}
	})
	t.Run("when acquisition is canceled, creates no assertion", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := (power.Native{}).Acquire(ctx, power.Options{Behaviors: power.IdleSystemSleep})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("when the owning app dies, releases its assertions", func(t *testing.T) {
		helper := startPowerHelper(t, "owner", power.Options{Behaviors: power.IdleSystemSleep})
		eventually(t, func() bool { return assertionsFor(t, helper.Process.Pid) != "" })
		if err := helper.Process.Kill(); err != nil {
			t.Fatal(err)
		}
		eventually(t, func() bool { return assertionsFor(t, helper.Process.Pid) == "" })
	})
}

func TestNativeProcessWatch(t *testing.T) {
	t.Run("when the process is already gone, completes immediately", func(t *testing.T) {
		helper := exec.Command("/usr/bin/true")
		if err := helper.Run(); err != nil {
			t.Fatal(err)
		}
		watch, err := (power.Native{}).WatchProcess(helper.Process.Pid)
		if err != nil {
			t.Fatal(err)
		}
		await(t, watch.Done())
		if err := watch.Close(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("when the watched process exits, reports completion", func(t *testing.T) {
		helper := startPowerHelper(t, "sleeper", power.Options{})
		watch, err := (power.Native{}).WatchProcess(helper.Process.Pid)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = watch.Close() })
		if err := helper.Process.Kill(); err != nil {
			t.Fatal(err)
		}
		await(t, watch.Done())
		if err := watch.Err(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("when canceled, leaves the watched process running", func(t *testing.T) {
		helper := startPowerHelper(t, "sleeper", power.Options{})
		watch, err := (power.Native{}).WatchProcess(helper.Process.Pid)
		if err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if err := watch.Close(); err != nil {
				t.Fatal(err)
			}
		}
		await(t, watch.Done())
		if err := helper.Process.Signal(syscall.Signal(0)); err != nil {
			t.Fatalf("watched process was affected: %v", err)
		}
	})
	t.Run("when exit races registration, still completes", func(t *testing.T) {
		for range 3 {
			helper := startPowerHelper(t, "sleeper", power.Options{})
			if err := helper.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			watch, err := (power.Native{}).WatchProcess(helper.Process.Pid)
			if err != nil {
				t.Fatal(err)
			}
			await(t, watch.Done())
			if err := watch.Close(); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func TestPowerHelper(t *testing.T) {
	mode := os.Getenv("KAFFEINATE_POWER_TEST_HELPER")
	if mode == "" {
		return
	}
	if mode == "startfailure" {
		var fileLimit syscall.Rlimit
		if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &fileLimit); err != nil {
			os.Exit(4)
		}
		fileLimit.Cur = 0
		if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &fileLimit); err != nil {
			os.Exit(5)
		}
		hold, err := (power.Native{}).Acquire(context.Background(), power.Options{Behaviors: power.IdleSystemSleep})
		if hold != nil || !errors.Is(err, syscall.EMFILE) {
			os.Exit(6)
		}
		fmt.Println("ready")
		os.Exit(0)
	}
	if mode == "owner" {
		var options power.Options
		if err := json.Unmarshal([]byte(os.Getenv("KAFFEINATE_POWER_TEST_OPTIONS")), &options); err != nil {
			os.Exit(2)
		}
		hold, err := (power.Native{}).Acquire(context.Background(), options)
		if err != nil {
			os.Exit(3)
		}
		defer hold.Release()
	}
	fmt.Println("ready")
	time.Sleep(time.Hour)
	os.Exit(0)
}

func acquire(t *testing.T, options power.Options) power.Hold {
	t.Helper()
	hold, err := (power.Native{}).Acquire(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := hold.Release(); err != nil {
			t.Error(err)
		}
	})
	return hold
}

func startPowerHelper(t *testing.T, mode string, options power.Options) *exec.Cmd {
	t.Helper()
	optionsJSON, err := json.Marshal(options)
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	helper := exec.CommandContext(ctx, executable, "-test.run=^TestPowerHelper$")
	helper.Env = append(os.Environ(), "KAFFEINATE_POWER_TEST_HELPER="+mode, "KAFFEINATE_POWER_TEST_OPTIONS="+string(optionsJSON))
	helper.Stderr = os.Stderr
	stdout, err := helper.StdoutPipe()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if err := helper.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = helper.Wait() })
	if !bufio.NewScanner(stdout).Scan() {
		t.Fatal("helper did not become ready")
	}
	return helper
}

func assertionsFor(t *testing.T, ownerPID int) string {
	t.Helper()
	output, err := exec.Command("/usr/bin/pmset", "-g", "assertions").Output()
	if err != nil {
		t.Fatal(err)
	}
	var currentHeader string
	var ownedHeaders []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "pid ") {
			currentHeader = line
		}
		if line == fmt.Sprintf("Created for PID: %d.", ownerPID) && strings.Contains(currentHeader, "(caffeinate)") {
			ownedHeaders = append(ownedHeaders, currentHeader)
		}
	}
	return strings.Join(ownedHeaders, "\n")
}

func eventually(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if condition() {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("condition did not become true before the deadline")
		case <-ticker.C:
		}
	}
}

func await(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("operation did not complete before the deadline")
	}
}
