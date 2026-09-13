package cli_test

import (
	"strings"
	"testing"
	"time"

	"kaffeinate/internal/cli"
	"kaffeinate/internal/power"
)

func TestCLIParse(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		arguments []string
		behaviors power.Behaviors
		duration  time.Duration
		pid       int
	}{
		{"when bare, requests indefinite idle prevention", nil, power.IdleSystemSleep, 0, 0},
		{"when given 28000 seconds, preserves the duration", []string{"-i", "-t", "28000"}, power.IdleSystemSleep, 28000 * time.Second, 0},
		{"when flags are combined, preserves both behaviors", []string{"-di"}, power.DisplaySleep | power.IdleSystemSleep, 0, 0},
		{"when flags are separate, preserves both behaviors", []string{"-d", "-i"}, power.DisplaySleep | power.IdleSystemSleep, 0, 0},
		{"when display is explicit, adds no other behavior", []string{"-d"}, power.DisplaySleep, 0, 0},
		{"when values are attached, reads duration and PID", []string{"-dit12", "-w123"}, power.DisplaySleep | power.IdleSystemSleep, 12 * time.Second, 123},
		{"when a value repeats, uses its last value", []string{"-t10", "-t20", "-w1", "-w2"}, power.IdleSystemSleep, 20 * time.Second, 2},
		{"when timeout is zero, has no explicit deadline", []string{"-u", "-t0"}, power.UserActivity, 0, 0},
		{"when user activity is timed, preserves its timeout", []string{"-u", "-t10"}, power.UserActivity, 10 * time.Second, 0},
		{"when flags end, accepts an empty remainder", []string{"-msu", "--"}, power.DiskIdleSleep | power.ACSystemSleep | power.UserActivity, 0, 0},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request, help, err := cli.Parse(testCase.arguments)
			if err != nil || help {
				t.Fatalf("parse: help=%v error=%v", help, err)
			}
			if request.PowerOptions.Behaviors != testCase.behaviors || request.PowerOptions.Duration != testCase.duration || request.WatchedPID != testCase.pid {
				t.Fatalf("request = %+v, want behaviors=%v duration=%v PID=%d", request, testCase.behaviors, testCase.duration, testCase.pid)
			}
		})
	}
}

func TestCLIValidation(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		arguments []string
		message   string
	}{
		{"when a value is missing, explains the missing value", []string{"-t"}, "requires a value"},
		{"when timeout is negative, rejects it", []string{"-t-1"}, "nonnegative integer"},
		{"when timeout is fractional, rejects it", []string{"-t1.5"}, "nonnegative integer"},
		{"when timeout overflows, reports invalid input", []string{"-t9223372037"}, "too large"},
		{"when a number overflows, rejects it", []string{"-t99999999999999999999999"}, "nonnegative integer"},
		{"when PID is zero, requires a positive PID", []string{"-w0"}, "PID must be"},
		{"when PID overflows, rejects it", []string{"-w2147483648"}, "PID must be"},
		{"when an option is unknown, points to help", []string{"-x"}, "use -h"},
		{"when wrapping a command, explains PID watching", []string{"make"}, "use -w PID"},
		{"when a utility follows the terminator, rejects it", []string{"--", "make"}, "wrapped commands"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, help, err := cli.Parse(testCase.arguments)
			if help || err == nil || !strings.Contains(err.Error(), testCase.message) {
				t.Fatalf("help=%v error=%v, want error containing %q", help, err, testCase.message)
			}
		})
	}
	t.Run("when help is requested, returns help successfully", func(t *testing.T) {
		_, help, err := cli.Parse([]string{"-h"})
		if err != nil || !help {
			t.Fatalf("help=%v error=%v", help, err)
		}
	})
}
