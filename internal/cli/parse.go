package cli

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"kaffeinate/internal/power"
	"kaffeinate/internal/session"
)

const Help = `Usage: kaffeinate [-dimsu] [-t seconds] [-w pid]

Start or replace Kaffeinate's background session, then return to your shell.
The menu bar app opens automatically. With no flags, prevent idle system sleep.

  -d  Keep the display awake.
  -i  Prevent idle system sleep.
  -m  Prevent disk idle sleep.
  -s  Prevent system sleep while on AC power.
  -u  Declare user activity (five seconds by default).
  -t  End after this many seconds; zero means no explicit timeout.
  -w  End when this PID exits, or when the timeout arrives first.
  -h  Show this help without opening the app.

Flags can be combined, for example -di. Each command replaces the active session.
Stop a session from the menu bar. Wrapped commands are not supported yet.

Examples:
  kaffeinate -i -t 28000
  kaffeinate -di -t 3600
  kaffeinate -i -w 12345
`

func Parse(arguments []string) (session.Request, bool, error) {
	var request session.Request
	for argumentIndex := 0; argumentIndex < len(arguments); argumentIndex++ {
		argument := arguments[argumentIndex]
		if argument == "--" {
			if argumentIndex+1 < len(arguments) {
				return request, false, wrappedCommandError()
			}
			break
		}
		if len(argument) < 2 || !strings.HasPrefix(argument, "-") {
			return request, false, wrappedCommandError()
		}
		for flagIndex := 1; flagIndex < len(argument); flagIndex++ {
			switch flag := argument[flagIndex]; flag {
			case 'h':
				return request, true, nil
			case 'd':
				request.PowerOptions.Behaviors |= power.DisplaySleep
			case 'i':
				request.PowerOptions.Behaviors |= power.IdleSystemSleep
			case 'm':
				request.PowerOptions.Behaviors |= power.DiskIdleSleep
			case 's':
				request.PowerOptions.Behaviors |= power.ACSystemSleep
			case 'u':
				request.PowerOptions.Behaviors |= power.UserActivity
			case 't', 'w':
				value := argument[flagIndex+1:]
				if value == "" {
					argumentIndex++
					if argumentIndex == len(arguments) {
						return request, false, fmt.Errorf("-%c requires a value", flag)
					}
					value = arguments[argumentIndex]
				}
				number, err := strconv.ParseInt(value, 10, 64)
				if err != nil || number < 0 {
					return request, false, fmt.Errorf("invalid -%c value %q: use a nonnegative integer", flag, value)
				}
				if flag == 't' {
					if number > math.MaxInt64/int64(time.Second) {
						return request, false, fmt.Errorf("timeout %q is too large", value)
					}
					request.PowerOptions.Duration = time.Duration(number) * time.Second
				} else {
					if number == 0 || number > math.MaxInt32 {
						return request, false, fmt.Errorf("PID must be between 1 and %d", math.MaxInt32)
					}
					request.WatchedPID = int(number)
				}
				flagIndex = len(argument)
			default:
				return request, false, fmt.Errorf("unknown option -%c; use -h for help", flag)
			}
		}
	}
	if request.PowerOptions.Behaviors == 0 {
		request.PowerOptions.Behaviors = power.IdleSystemSleep
	}
	return request, false, request.Validate()
}

func wrappedCommandError() error {
	return fmt.Errorf("wrapped commands are not supported; use -w PID to watch an existing process")
}
