package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func LaunchApp(ctx context.Context) error {
	executablePath, err := os.Executable()
	if err != nil {
		return err
	}
	executablePath, err = filepath.EvalSymlinks(executablePath)
	if err != nil {
		return fmt.Errorf("locate installed CLI: %w", err)
	}
	bundlePath := filepath.Clean(filepath.Join(filepath.Dir(executablePath), "../../.."))
	appExecutable := filepath.Join(bundlePath, "Contents", "MacOS", "Kaffeinate")
	info, err := os.Stat(appExecutable)
	if !strings.HasSuffix(bundlePath, ".app") || err != nil || info.IsDir() || info.Mode().Perm()&0111 == 0 {
		return fmt.Errorf("cannot locate Kaffeinate.app beside this CLI; run make build and use the bundled or installed kaffeinate command")
	}
	// Launch Services can reuse a just-exited instance without -n.
	// The app's instance lock handles concurrent launches.
	output, err := exec.CommandContext(ctx, "/usr/bin/open", "-g", "-n", bundlePath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("open Kaffeinate.app: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
