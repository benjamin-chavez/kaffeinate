package main

import (
	"context"
	"os"
	"time"

	"kaffeinate/internal/cli"
	"kaffeinate/internal/control"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	runner := cli.Runner{
		Client: control.Client{SocketPath: control.SocketPath(control.DefaultDirectory())},
		Launch: cli.LaunchApp,
	}
	os.Exit(runner.Run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}
