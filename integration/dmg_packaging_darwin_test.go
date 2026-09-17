package integration_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDMGCopy(t *testing.T) {
	t.Run("when copied, the app remains usable after the image is detached", func(t *testing.T) {
		fixture := newDiskImageFixture(t)
		fixture.environment = append(fixture.environment, "DMG_TEST_UNRELATED=true")
		output, err := fixture.run("copy-dmg-app.sh", fixture.imagePath, fixture.copyPath)
		if err != nil {
			t.Fatalf("copy image: %v: %s", err, output)
		}
		if _, err := os.Stat(filepath.Join(fixture.copyPath, "Contents", "Info.plist")); err != nil {
			t.Fatalf("independent app copy: %v", err)
		}
		fixture.requireDetached(t)
	})
	for _, testCase := range []struct{ result, message string }{
		{"attach", "attachment failed"},
		{"copy", "copy failed"},
		{"detach", "Retained image workspace"},
		{"discovery", "Retained image workspace"},
		{"verify", "image verification failed"},
	} {
		t.Run("when "+testCase.result+" fails, reports failure and handles its attachment", func(t *testing.T) {
			fixture := newDiskImageFixture(t)
			fixture.environment = append(fixture.environment, "DMG_TEST_RESULT="+testCase.result)
			output, err := fixture.run("copy-dmg-app.sh", fixture.imagePath, fixture.copyPath)
			if err == nil || !strings.Contains(output, testCase.message) {
				t.Fatalf("wanted %q, got %v: %s", testCase.message, err, output)
			}
			if testCase.result == "detach" || testCase.result == "discovery" {
				fixture.requireRetained(t)
			} else {
				fixture.requireDetached(t)
			}
			if _, err := os.Stat(fixture.imagePath); err != nil {
				t.Fatalf("release image was removed: %v", err)
			}
		})
	}
	for _, attachmentMounted := range []bool{false, true} {
		t.Run("when already attached, leaves the existing device alone", func(t *testing.T) {
			fixture := newDiskImageFixture(t)
			mountDirectory := ""
			if attachmentMounted {
				mountDirectory = filepath.Join(fixture.rootDirectory, "user mounted image")
				if err := os.Mkdir(mountDirectory, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			attachmentPlist := "<plist version=\"1.0\"><dict><key>images</key><array><dict><key>image-path</key><string>" + fixture.imagePath + "</string><key>system-entities</key><array><dict><key>dev-entry</key><string>/dev/disk99</string>"
			if attachmentMounted {
				attachmentPlist += "<key>mount-point</key><string>" + mountDirectory + "</string>"
			}
			attachmentPlist += "</dict></array></dict></array></dict></plist>"
			for name, contents := range map[string]string{"disk.plist": attachmentPlist, "mounted": mountDirectory} {
				if err := os.WriteFile(filepath.Join(fixture.stateDirectory, name), []byte(contents), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			output, err := fixture.run("copy-dmg-app.sh", fixture.imagePath, fixture.copyPath)
			if err == nil || !strings.Contains(output, "already attached") {
				t.Fatalf("existing attachment should stop verification: %v: %s", err, output)
			}
			if _, err := os.Stat(filepath.Join(fixture.stateDirectory, "mounted")); err != nil {
				t.Fatalf("existing device was detached: %v", err)
			}
		})
	}
	for _, invalidContent := range []string{"Applications", ".DS_Store", "extra.txt"} {
		t.Run("when installer content is invalid at "+invalidContent+", refuses the copy", func(t *testing.T) {
			fixture := newDiskImageFixture(t)
			contentPath := filepath.Join(fixture.stateDirectory, "payload", invalidContent)
			if invalidContent == "extra.txt" {
				if err := os.WriteFile(contentPath, []byte("unexpected"), 0o644); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Remove(contentPath); err != nil {
					t.Fatal(err)
				}
				if invalidContent == "Applications" {
					if err := os.Symlink("/tmp", contentPath); err != nil {
						t.Fatal(err)
					}
				}
			}
			output, err := fixture.run("copy-dmg-app.sh", fixture.imagePath, fixture.copyPath)
			if err == nil {
				t.Fatalf("invalid installer should fail: %s", output)
			}
			if _, err := os.Stat(fixture.copyPath); !os.IsNotExist(err) {
				t.Fatalf("invalid image produced an app copy: %v", err)
			}
			fixture.requireDetached(t)
		})
	}
}

func TestDMGPackaging(t *testing.T) {
	t.Run("when the output image is attached, preserves its backing file", func(t *testing.T) {
		fixture := newDiskImageFixture(t)
		attachmentPlist := "<plist version=\"1.0\"><dict><key>images</key><array><dict><key>image-path</key><string>" + fixture.imagePath + "</string><key>system-entities</key><array><dict><key>dev-entry</key><string>/dev/disk99</string></dict></array></dict></array></dict></plist>"
		for name, contents := range map[string]string{"disk.plist": attachmentPlist, "mounted": ""} {
			if err := os.WriteFile(filepath.Join(fixture.stateDirectory, name), []byte(contents), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		bundlePath := filepath.Join(fixture.stateDirectory, "payload", "Kaffeinate.app")
		output, err := fixture.run("create-dmg.sh", bundlePath, fixture.imagePath)
		if err == nil || !strings.Contains(output, "already attached") {
			t.Fatalf("attached output should prevent packaging: %v: %s", err, output)
		}
		imageBytes, err := os.ReadFile(fixture.imagePath)
		if err != nil || string(imageBytes) != "disk image" {
			t.Fatalf("attached output was changed: %v: %s", err, imageBytes)
		}
		if _, err := os.Stat(filepath.Join(fixture.stateDirectory, "mounted")); err != nil {
			t.Fatalf("existing attachment was detached: %v", err)
		}
	})
	for _, testCase := range []struct{ result, message string }{
		{"pass", ""},
		{"finder", "Finder access denied"},
		{"layout", "Finder did not save"},
		{"attach", "attachment failed"},
		{"detach", "Retained image workspace"},
		{"verify", "image verification failed"},
	} {
		t.Run("when the result is "+testCase.result+", publishes only a completed image", func(t *testing.T) {
			fixture := newDiskImageFixture(t)
			fixture.environment = append(fixture.environment, "DMG_TEST_RESULT="+testCase.result)
			outputPath := filepath.Join(fixture.rootDirectory, "installer.dmg")
			bundlePath := filepath.Join(fixture.stateDirectory, "payload", "Kaffeinate.app")
			output, err := fixture.run("create-dmg.sh", bundlePath, outputPath)
			if testCase.result == "pass" {
				if err != nil {
					t.Fatalf("package image: %v: %s", err, output)
				}
				if _, err := os.Stat(outputPath); err != nil {
					t.Fatalf("missing completed image: %v", err)
				}
			} else {
				if err == nil || !strings.Contains(output, testCase.message) {
					t.Fatalf("wanted %q, got %v: %s", testCase.message, err, output)
				}
				if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
					t.Fatalf("failed packaging exposed an output image: %v", err)
				}
			}
			if testCase.result == "detach" {
				fixture.requireRetained(t)
			} else {
				fixture.requireDetached(t)
			}
		})
	}
}

type diskImageFixture struct {
	rootDirectory  string
	stateDirectory string
	imagePath      string
	copyPath       string
	environment    []string
}

func newDiskImageFixture(t *testing.T) diskImageFixture {
	t.Helper()
	rootDirectory := t.TempDir()
	toolDirectory := filepath.Join(rootDirectory, "native tools")
	stateDirectory := filepath.Join(rootDirectory, "disk state")
	temporaryDirectory := filepath.Join(rootDirectory, "temporary files")
	for _, directory := range []string{toolDirectory, stateDirectory, temporaryDirectory} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeDiskImageFakes(t, toolDirectory)
	payloadDirectory := filepath.Join(stateDirectory, "payload")
	for _, directory := range []string{"Kaffeinate.app/Contents", ".background"} {
		if err := os.MkdirAll(filepath.Join(payloadDirectory, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, contents := range map[string][]byte{
		"Kaffeinate.app/Contents/Info.plist": []byte("<plist version=\"1.0\"><dict/></plist>"),
		".DS_Store":                          []byte("saved Finder layout"),
	} {
		if err := os.WriteFile(filepath.Join(payloadDirectory, name), contents, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	backgroundBytes, err := os.ReadFile("../packaging/macos/dmg-background.png")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(payloadDirectory, ".background", "background.png"), backgroundBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/Applications", filepath.Join(payloadDirectory, "Applications")); err != nil {
		t.Fatal(err)
	}
	fixture := diskImageFixture{
		rootDirectory:  rootDirectory,
		stateDirectory: stateDirectory,
		imagePath:      filepath.Join(rootDirectory, "download with spaces.dmg"),
		copyPath:       filepath.Join(rootDirectory, "installed app", "Kaffeinate.app"),
		environment: append(os.Environ(),
			"PATH="+toolDirectory+":/usr/bin:/bin:/usr/sbin:/sbin",
			"TMPDIR="+temporaryDirectory,
			"DMG_TEST_STATE="+stateDirectory),
	}
	if err := os.WriteFile(fixture.imagePath, []byte("disk image"), 0o644); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func (fixture diskImageFixture) run(scriptName string, arguments ...string) (string, error) {
	scriptPath, err := filepath.Abs(filepath.Join("..", "scripts", scriptName))
	if err != nil {
		return "", err
	}
	command := exec.Command("/bin/bash", append([]string{scriptPath}, arguments...)...)
	command.Dir = fixture.rootDirectory
	command.Env = fixture.environment
	output, err := command.CombinedOutput()
	return string(output), err
}

func (fixture diskImageFixture) requireDetached(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(fixture.stateDirectory, "mounted")); !os.IsNotExist(err) {
		t.Fatalf("image should have been detached: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(fixture.rootDirectory, "temporary files"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary image resources remain: %v", entries)
	}
}

func (fixture diskImageFixture) requireRetained(t *testing.T) {
	t.Helper()
	mountBytes, err := os.ReadFile(filepath.Join(fixture.stateDirectory, "mounted"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(string(mountBytes)); err != nil {
		t.Fatalf("cleanup deleted through an active mount: %v", err)
	}
}

func writeDiskImageFakes(t *testing.T, toolDirectory string) {
	t.Helper()
	for toolName, toolScript := range map[string]string{
		"hdiutil": `empty_images() {
  printf '%s\n' '<plist version="1.0"><dict><key>images</key><array/></dict></plist>'
}
case $1 in
info)
  if [[ -f "$DMG_TEST_STATE/mounted" ]]; then
    if [[ ${DMG_TEST_RESULT:-} == discovery ]]; then echo invalid; exit 0; fi
    plutil -convert xml1 -o "$DMG_TEST_STATE/reported.plist" "$DMG_TEST_STATE/disk.plist"
  else
    empty_images > "$DMG_TEST_STATE/reported.plist"
  fi
  if [[ ${DMG_TEST_UNRELATED:-} == true ]]; then
    image_count=$(plutil -extract images raw -o - "$DMG_TEST_STATE/reported.plist")
    plutil -insert "images.$image_count" -xml '<dict><key>image-path</key><string>/unavailable/old/image.dmg</string><key>system-entities</key><array><dict><key>dev-entry</key><string>/dev/disk98</string></dict></array></dict>' "$DMG_TEST_STATE/reported.plist"
  fi
  plutil -convert xml1 -o - "$DMG_TEST_STATE/reported.plist" ;;
create)
  shift
  while [[ $# -gt 1 ]]; do
    case $1 in -srcfolder) source_directory=$2; shift ;; esac
    shift
  done
  rm -rf "$DMG_TEST_STATE/payload"
  ditto "$source_directory" "$DMG_TEST_STATE/payload"
  printf image > "$1" ;;
attach)
  image_path=$2
  shift 2
  while [[ $# -gt 0 ]]; do
    if [[ $1 == -mountpoint ]]; then mount_directory=$2; shift; fi
    shift
  done
  ditto "$DMG_TEST_STATE/payload" "$mount_directory"
  empty_images > "$DMG_TEST_STATE/disk.plist"
  plutil -insert images.0 -xml '<dict/>' "$DMG_TEST_STATE/disk.plist"
  plutil -insert images.0.image-path -string "$image_path" "$DMG_TEST_STATE/disk.plist"
  plutil -insert images.0.system-entities -xml '<array><dict><key>dev-entry</key><string>/dev/disk99</string></dict></array>' "$DMG_TEST_STATE/disk.plist"
  plutil -insert images.0.system-entities.0.mount-point -string "$mount_directory" "$DMG_TEST_STATE/disk.plist"
  printf '%s' "$mount_directory" > "$DMG_TEST_STATE/mounted"
  if [[ ${DMG_TEST_RESULT:-} == attach ]]; then echo 'attachment failed' >&2; exit 1; fi
  plutil -extract images.0 xml1 -o - "$DMG_TEST_STATE/disk.plist" ;;
detach)
  if [[ $2 != /dev/disk99 ]]; then echo 'attempted to detach an unrelated device' >&2; exit 1; fi
  if [[ ${DMG_TEST_RESULT:-} == detach ]]; then echo 'device busy' >&2; exit 1; fi
  mount_directory=$(<"$DMG_TEST_STATE/mounted")
  rm -rf "$mount_directory"
  rm "$DMG_TEST_STATE/mounted" ;;
convert)
  test ! -f "$DMG_TEST_STATE/mounted"
  while [[ $1 != -o ]]; do shift; done
  printf image > "$2" ;;
verify)
  if [[ ${DMG_TEST_RESULT:-} == verify ]]; then echo 'image verification failed' >&2; exit 1; fi ;;
imageinfo) echo UDZO ;;
*) echo "unexpected disk command: $*" >&2; exit 1 ;;
esac
`,
		"osascript": `if [[ ${DMG_TEST_RESULT:-} == finder ]]; then echo 'Finder access denied' >&2; exit 1; fi
if [[ ${DMG_TEST_RESULT:-} != layout ]]; then printf layout > "$2/.DS_Store"; fi
`,
		"sleep": "exit 0\n",
		"ditto": `if [[ ${DMG_TEST_RESULT:-} == copy && $1 == */volume/Kaffeinate.app ]]; then echo 'copy failed' >&2; exit 1; fi
/usr/bin/ditto "$@"
`,
	} {
		if err := os.WriteFile(filepath.Join(toolDirectory, toolName), []byte("#!/bin/bash\nset -euo pipefail\n"+toolScript), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}
