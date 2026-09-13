package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"kaffeinate/internal/control"
	"kaffeinate/internal/power"
	"kaffeinate/internal/session"
	"kaffeinate/internal/tray"
)

func main() {
	controller := session.NewController(power.Native{})
	server, err := control.Listen(context.Background(), control.DefaultDirectory(), controller)
	if err != nil {
		_ = controller.Close()
		if !errors.Is(err, control.ErrAlreadyRunning) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	var shutdownOnce sync.Once
	shutdown := func() {
		shutdownOnce.Do(func() {
			requestErr := server.StopRequests()
			if err := controller.Close(); err != nil {
				fmt.Fprintln(os.Stderr, errors.Join(requestErr, err))
				os.Exit(1)
			}
			if err := errors.Join(requestErr, server.Close()); err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
		})
	}
	defer shutdown()
	tray.Run(controller, shutdown)
}
