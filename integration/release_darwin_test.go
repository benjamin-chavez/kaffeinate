package integration_test

import (
	"crypto/sha256"
	"debug/macho"
	"encoding/hex"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"kaffeinate/internal/control"
)

func TestReleaseDiskImage(t *testing.T) {
	releaseDirectory := os.Getenv("KAFFEINATE_TEST_RELEASE_DIR")
	if releaseDirectory == "" {
		t.Skip("set KAFFEINATE_TEST_RELEASE_DIR to check a universal release")
	}
	imageName := "Kaffeinate-macOS-universal.dmg"
	imagePath := filepath.Join(releaseDirectory, imageName)
	imageFile, err := os.Open(imagePath)
	if err != nil {
		t.Fatal(err)
	}
	defer imageFile.Close()
	imageHash := sha256.New()
	if _, err := io.Copy(imageHash, imageFile); err != nil {
		t.Fatal(err)
	}
	checksumBytes, err := os.ReadFile(filepath.Join(releaseDirectory, "SHA256SUMS.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !t.Run("when downloaded, its checksum identifies the complete image", func(t *testing.T) {
		checksumFields := strings.Fields(string(checksumBytes))
		if len(checksumFields) != 2 || checksumFields[0] != hex.EncodeToString(imageHash.Sum(nil)) || checksumFields[1] != imageName {
			t.Fatalf("checksum file does not identify this image: %s", checksumBytes)
		}
	}) {
		return
	}
	installationDirectory := filepath.Join(t.TempDir(), "download with spaces")
	bundlePath := filepath.Join(installationDirectory, "Kaffeinate.app")
	if !t.Run("when mounted, contains a drag installer and copies an independent app", func(t *testing.T) {
		releaseCommand(t, "/usr/bin/env", "PATH=/usr/bin:/bin:/usr/sbin:/sbin", "/bin/bash",
			"../scripts/copy-dmg-app.sh", imagePath, bundlePath)
	}) {
		return
	}
	plistPath := filepath.Join(bundlePath, "Contents", "Info.plist")
	minimumOS := plistValue(t, plistPath, "LSMinimumSystemVersion")
	t.Run("when installed, retains its version and menu-only metadata", func(t *testing.T) {
		for _, key := range []string{"CFBundleIdentifier", "CFBundleVersion", "CFBundleShortVersionString", "LSMinimumSystemVersion", "LSUIElement"} {
			want := plistValue(t, "../packaging/macos/Info.plist", key)
			if got := plistValue(t, plistPath, key); got != want {
				t.Fatalf("%s = %s, want %s", key, got, want)
			}
		}
		iconName := plistValue(t, plistPath, "CFBundleIconFile")
		if _, err := os.Stat(filepath.Join(bundlePath, "Contents", "Resources", iconName)); err != nil {
			t.Fatalf("application icon: %v", err)
		}
	})
	for _, executable := range []struct{ name, relativePath string }{
		{"app", "Contents/MacOS/Kaffeinate"},
		{"CLI", "Contents/Resources/bin/kaffeinate"},
	} {
		t.Run("when copied, the "+executable.name+" supports both Mac architectures", func(t *testing.T) {
			executablePath := filepath.Join(bundlePath, executable.relativePath)
			info, err := os.Stat(executablePath)
			if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
				t.Fatalf("executable permissions: %v: %v", info, err)
			}
			fatBinary, err := macho.OpenFat(executablePath)
			if err != nil {
				t.Fatalf("universal binary: %v", err)
			}
			defer fatBinary.Close()
			if len(fatBinary.Arches) != 2 {
				t.Fatalf("architectures = %d, want 2", len(fatBinary.Arches))
			}
			architectures := make(map[macho.Cpu]bool)
			for _, architecture := range fatBinary.Arches {
				architectures[architecture.Cpu] = true
				checkMinimumOS(t, architecture.File, minimumOS)
			}
			if !architectures[macho.CpuArm64] || !architectures[macho.CpuAmd64] {
				t.Fatalf("missing Apple Silicon or Intel support: %v", architectures)
			}
			releaseCommand(t, "/usr/bin/codesign", "--verify", "--strict", executablePath)
		})
	}
	t.Run("when copied, its bundle signature remains valid", func(t *testing.T) {
		releaseCommand(t, "/usr/bin/codesign", "--verify", "--deep", "--strict", bundlePath)
	})
	cliPath := filepath.Join(bundlePath, "Contents", "Resources", "bin", "kaffeinate")
	for _, architecture := range []string{"arm64", "x86_64"} {
		t.Run("when run as "+architecture+", the CLI needs no developer tools", func(t *testing.T) {
			requireArchitecture(t, architecture)
			command := exec.Command("/usr/bin/arch", "-"+architecture, cliPath, "-h")
			command.Dir = installationDirectory
			command.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "HOME=" + os.Getenv("HOME")}
			output, err := command.CombinedOutput()
			if err != nil || !strings.Contains(string(output), "Usage: kaffeinate") {
				t.Fatalf("CLI help: %v: %s", err, output)
			}
		})
	}
}

func TestReleaseAppArchitectures(t *testing.T) {
	bundlePath := os.Getenv("KAFFEINATE_TEST_APP_BUNDLE")
	if bundlePath == "" {
		t.Skip("set KAFFEINATE_TEST_APP_BUNDLE to test app architectures")
	}
	if len(appPIDs()) != 0 {
		t.Skip("quit the existing Kaffeinate application before testing architectures")
	}
	bundlePath, err := filepath.Abs(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, architecture := range []string{"arm64", "x86_64"} {
		t.Run("when launched as "+architecture+", owns and releases a timed session", func(t *testing.T) {
			requireArchitecture(t, architecture)
			appPath := filepath.Join(bundlePath, "Contents", "MacOS", "Kaffeinate")
			command := exec.Command("/usr/bin/arch", "-"+architecture, appPath)
			command.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "HOME=" + os.Getenv("HOME")}
			command.Stderr = os.Stderr
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			appExit := make(chan error, 1)
			go func() { appExit <- command.Wait() }()
			t.Cleanup(func() {
				_ = command.Process.Signal(syscall.SIGTERM)
				select {
				case err := <-appExit:
					if err != nil && !t.Failed() {
						t.Errorf("app did not quit cleanly: %v", err)
					}
				case <-time.After(5 * time.Second):
					_ = command.Process.Kill()
					select {
					case <-appExit:
					case <-time.After(3 * time.Second):
						t.Error("app did not exit after being killed")
					}
					t.Error("app did not respond to normal termination")
				}
				cleanupOwnedApps(t, bundlePath)
			})
			waitFor(t, func() bool {
				connection, err := net.DialTimeout("unix", control.SocketPath(control.DefaultDirectory()), 100*time.Millisecond)
				if err != nil {
					return false
				}
				_ = connection.Close()
				return true
			})
			cliPath := filepath.Join(bundlePath, "Contents", "Resources", "bin", "kaffeinate")
			runCLI(t, cliPath, filepath.Dir(bundlePath), "-i", "-t2")
			waitFor(t, func() bool {
				return strings.Contains(ownedAssertions(t, command.Process.Pid), "PreventUserIdleSystemSleep")
			})
			waitFor(t, func() bool {
				return ownedAssertions(t, command.Process.Pid) == "" && len(nativePIDs(command.Process.Pid)) == 0
			})
			if !processRunning(command.Process.Pid) {
				t.Fatal("application exited with its timed session")
			}
		})
	}
}

func requireArchitecture(t *testing.T, architecture string) {
	t.Helper()
	if architecture == "arm64" {
		capability, err := exec.Command("/usr/sbin/sysctl", "-n", "hw.optional.arm64").Output()
		if err != nil || strings.TrimSpace(string(capability)) != "1" {
			t.Skip("Apple Silicon execution requires an Apple Silicon host")
		}
	} else if runtime.GOARCH == "arm64" {
		if err := exec.Command("/usr/sbin/pkgutil", "--pkg-info", "com.apple.pkg.RosettaUpdateAuto").Run(); err != nil {
			t.Skip("Intel execution requires Rosetta to already be installed")
		}
	}
}

func releaseCommand(t *testing.T, name string, arguments ...string) string {
	t.Helper()
	output, err := exec.Command(name, arguments...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v: %s", name, arguments, err, output)
	}
	return strings.TrimSpace(string(output))
}

func plistValue(t *testing.T, plistPath, key string) string {
	t.Helper()
	return releaseCommand(t, "/usr/libexec/PlistBuddy", "-c", "Print :"+key, plistPath)
}

func checkMinimumOS(t *testing.T, binaryFile *macho.File, declaredMinimum string) {
	t.Helper()
	var minimumParts [3]uint32
	for index, part := range strings.Split(declaredMinimum, ".") {
		if index >= len(minimumParts) {
			t.Fatalf("invalid minimum OS: %s", declaredMinimum)
		}
		value, err := strconv.ParseUint(part, 10, 16)
		if err != nil {
			t.Fatal(err)
		}
		minimumParts[index] = uint32(value)
	}
	declaredVersion := minimumParts[0]<<16 | minimumParts[1]<<8 | minimumParts[2]
	const versionMinMacOSX = 0x24
	const buildVersion = 0x32
	for _, load := range binaryFile.Loads {
		loadBytes := load.Raw()
		if len(loadBytes) < 16 {
			continue
		}
		var minimumVersion uint32
		switch binaryFile.ByteOrder.Uint32(loadBytes[:4]) {
		case versionMinMacOSX:
			minimumVersion = binaryFile.ByteOrder.Uint32(loadBytes[8:12])
		case buildVersion:
			minimumVersion = binaryFile.ByteOrder.Uint32(loadBytes[12:16])
		default:
			continue
		}
		if minimumVersion > declaredVersion {
			t.Fatalf("binary requires macOS %d.%d.%d, above the advertised %s", minimumVersion>>16, minimumVersion>>8&255, minimumVersion&255, declaredMinimum)
		}
		return
	}
	t.Fatal("binary has no minimum macOS version")
}
