package tray

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/getlantern/systray"
	"kaffeinate/internal/power"
	"kaffeinate/internal/session"
)

type menuCommand struct {
	toggle   bool
	duration time.Duration
}

func Run(controller *session.Controller, shutdown func()) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	systray.Run(func() {
		statusItem := systray.AddMenuItem("Kaffeinate: Inactive", "")
		statusItem.Disable()
		behaviorItem := systray.AddMenuItem("", "")
		behaviorItem.Disable()
		processItem := systray.AddMenuItem("", "")
		processItem.Disable()
		errorItem := systray.AddMenuItem("", "")
		errorItem.Disable()
		systray.AddSeparator()
		toggleItem := systray.AddMenuItemCheckbox("Keep Awake", "Start or stop sleep prevention", false)
		timerItem := systray.AddMenuItem("Keep Awake For", "Start a new timed session")
		var presetItems []*systray.MenuItem
		for _, title := range []string{"15 Minutes", "30 Minutes", "1 Hour", "2 Hours"} {
			presetItems = append(presetItems, timerItem.AddSubMenuItem(title, ""))
		}
		systray.AddSeparator()
		quitItem := systray.AddMenuItem("Quit Kaffeinate", "Stop the session and quit")
		terminationSignals := make(chan os.Signal, 1)
		signal.Notify(terminationSignals, syscall.SIGINT, syscall.SIGTERM)
		commands := make(chan menuCommand, 8)
		go runCommands(ctx, controller, commands)
		go forwardClicks(ctx, toggleItem.ClickedCh, commands, menuCommand{toggle: true})
		for index, duration := range []time.Duration{15 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour} {
			go forwardClicks(ctx, presetItems[index].ClickedCh, commands, menuCommand{duration: duration})
		}
		go func() {
			defer signal.Stop(terminationSignals)
			select {
			case <-ctx.Done():
				return
			case <-quitItem.ClickedCh:
			case <-terminationSignals:
			}
			systray.Quit()
		}()
		var previousPresentation Presentation
		render := func() {
			snapshot := controller.Current()
			presentation := Present(snapshot)
			if presentation == previousPresentation {
				return
			}
			previousPresentation = presentation
			setIcon(presentation.Active)
			systray.SetTooltip(presentation.StatusText + "\n" + presentation.BehaviorText)
			statusItem.SetTitle(presentation.StatusText)
			setDetail(behaviorItem, presentation.BehaviorText)
			setDetail(processItem, presentation.ProcessText)
			setDetail(errorItem, presentation.ErrorText)
			errorItem.SetTooltip(snapshot.Error)
			if presentation.Active {
				toggleItem.Check()
			} else {
				toggleItem.Uncheck()
			}
			if presentation.Busy {
				toggleItem.Disable()
				timerItem.Disable()
			} else {
				toggleItem.Enable()
				timerItem.Enable()
			}
		}
		render()
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case _, ok := <-controller.Changes():
					if !ok {
						return
					}
					render()
				}
			}
		}()
	}, func() {
		cancel()
		shutdown()
	})
}

func forwardClicks(ctx context.Context, clicks <-chan struct{}, commands chan<- menuCommand, command menuCommand) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-clicks:
			select {
			case <-ctx.Done():
				return
			case commands <- command:
			}
		}
	}
}

func runCommands(ctx context.Context, controller *session.Controller, commands <-chan menuCommand) {
	for {
		select {
		case <-ctx.Done():
			return
		case command := <-commands:
			if command.toggle && controller.Current().Behaviors != 0 {
				_, _ = controller.Stop(ctx)
			} else {
				_, _ = controller.Start(ctx, session.Request{PowerOptions: power.Options{Behaviors: power.IdleSystemSleep, Duration: command.duration}})
			}
		}
	}
}

func setDetail(item *systray.MenuItem, text string) {
	if text == "" {
		item.Hide()
		return
	}
	item.SetTitle(text)
	item.Show()
}
