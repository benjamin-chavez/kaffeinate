package integration_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestReleaseWorkflowCheck(t *testing.T) {
	for _, checkResult := range []string{"pass", "architecture-skip"} {
		t.Run(checkResult, func(t *testing.T) {
			fixture := newReleaseWorkflowFixture(t)
			output, err := fixture.run("check", "RELEASE_TEST_RESULT="+checkResult,
				"KAFFEINATE_TEST_RELEASE_DIR=stale", "KAFFEINATE_TEST_APP_BUNDLE=stale")
			if err != nil {
				t.Fatalf("release check: %v: %s", err, output)
			}
			if !strings.Contains(output, "Release checks passed") {
				t.Fatalf("missing verification result: %s", output)
			}
			if _, err := os.Stat(filepath.Join(fixture.releaseDirectory, "Kaffeinate-macOS-universal.dmg")); err != nil {
				t.Fatal(err)
			}
			commandLog, err := os.ReadFile(fixture.commandLogPath)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(commandLog), "github") {
				t.Fatalf("local verification contacted GitHub: %s", commandLog)
			}
			fixture.requireTemporaryCleanup(t)
		})
	}
}

func TestReleaseWorkflowDraft(t *testing.T) {
	fixture := newReleaseWorkflowFixture(t)
	releaseCommit := fixture.git(t, "rev-parse", "HEAD")
	output, err := fixture.run("draft")
	if err != nil {
		t.Fatalf("release draft: %v: %s", err, output)
	}
	argumentBytes, err := os.ReadFile(fixture.draftArgumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	draftArguments := strings.Split(strings.TrimSuffix(string(argumentBytes), "\x00"), "\x00")
	for _, argument := range []string{"v1.2.3", "--draft", filepath.Join(fixture.releaseDirectory, "Kaffeinate-macOS-universal.dmg"), filepath.Join(fixture.releaseDirectory, "SHA256SUMS.txt")} {
		if !slices.Contains(draftArguments, argument) {
			t.Errorf("draft is missing %q: %v", argument, draftArguments)
		}
	}
	var uploadedAssets []string
	for argumentIndex := 3; argumentIndex < len(draftArguments); argumentIndex++ {
		switch draftArguments[argumentIndex] {
		case "--repo", "--target", "--title", "--notes":
			argumentIndex++
		case "--draft", "--generate-notes":
		default:
			uploadedAssets = append(uploadedAssets, draftArguments[argumentIndex])
		}
	}
	slices.Sort(uploadedAssets)
	if !slices.Equal(uploadedAssets, []string{filepath.Join(fixture.releaseDirectory, "Kaffeinate-macOS-universal.dmg"), filepath.Join(fixture.releaseDirectory, "SHA256SUMS.txt")}) {
		t.Fatalf("unexpected uploaded assets: %v", uploadedAssets)
	}
	if _, err := os.Stat(filepath.Join(fixture.releaseDirectory, "Kaffeinate-macOS-universal.zip")); err != nil {
		t.Fatalf("existing local ZIP should remain available: %v", err)
	}
	for flag, value := range map[string]string{"--target": releaseCommit, "--repo": "release-tests/kaffeinate"} {
		argumentIndex := slices.Index(draftArguments, flag)
		if argumentIndex < 0 || argumentIndex+1 >= len(draftArguments) || draftArguments[argumentIndex+1] != value {
			t.Errorf("draft %s should be %q: %v", flag, value, draftArguments)
		}
	}
	if !strings.Contains(output, "gh release edit v1.2.3 --repo release-tests/kaffeinate --draft=false --latest") {
		t.Fatalf("missing publication instructions: %s", output)
	}
	fixture.requireTemporaryCleanup(t)
}

func TestReleaseWorkflowStopsBeforeUpload(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		environment string
		message     string
	}{
		{"running app", "RELEASE_TEST_PROCESS=running", "Quit Kaffeinate"},
		{"unavailable process list", "RELEASE_TEST_PROCESS=error", "Could not check"},
		{"unmerged commit", "RELEASE_TEST_REMOTE=unmerged", "latest merged main commit"},
		{"existing release", "RELEASE_TEST_REMOTE=release", "already has a release or tag"},
		{"existing tag", "RELEASE_TEST_REMOTE=tag", "already has a release or tag"},
		{"GitHub error", "RELEASE_TEST_REMOTE=error", "GitHub unavailable"},
		{"failed build", "RELEASE_TEST_RESULT=build", "build failed"},
		{"failed tests", "RELEASE_TEST_RESULT=unit", "unit tests failed"},
		{"failed vet", "RELEASE_TEST_RESULT=vet", "vet failed"},
		{"incorrect checksum", "RELEASE_TEST_RESULT=checksum", "checksum does not match"},
		{"failed image copy", "DMG_TEST_RESULT=copy", "copy failed"},
		{"failed app checks", "RELEASE_TEST_RESULT=integration", "app checks failed"},
		{"skipped app checks", "RELEASE_TEST_RESULT=skip", "required integration check was skipped"},
		{"source changes during checks", "RELEASE_TEST_RESULT=changed", "Commit and merge your changes"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newReleaseWorkflowFixture(t)
			output, err := fixture.run("draft", testCase.environment)
			if err == nil || !strings.Contains(output, testCase.message) {
				t.Fatalf("wanted failure containing %q, got %v: %s", testCase.message, err, output)
			}
			if _, err := os.Stat(fixture.draftArgumentsPath); !os.IsNotExist(err) {
				t.Fatalf("draft upload should not have run: %v", err)
			}
			fixture.requireTemporaryCleanup(t)
		})
	}
}

func TestReleaseWorkflowUploadFailure(t *testing.T) {
	fixture := newReleaseWorkflowFixture(t)
	output, err := fixture.run("draft", "RELEASE_TEST_RESULT=upload")
	if err == nil || !strings.Contains(output, "upload failed") || strings.Contains(output, "Publish when ready") {
		t.Fatalf("wanted upload failure without publication instructions: %v: %s", err, output)
	}
	fixture.requireTemporaryCleanup(t)
}

func TestReleaseWorkflowRetainsBusyImage(t *testing.T) {
	fixture := newReleaseWorkflowFixture(t)
	output, err := fixture.run("draft", "DMG_TEST_RESULT=detach")
	if err == nil || !strings.Contains(output, "Retained image workspace") {
		t.Fatalf("busy image should fail with recovery instructions: %v: %s", err, output)
	}
	mountBytes, err := os.ReadFile(filepath.Join(fixture.diskStateDirectory, "mounted"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(string(mountBytes)); err != nil {
		t.Fatalf("workflow cleanup removed an active mount: %v", err)
	}
	if _, err := os.Stat(fixture.draftArgumentsPath); !os.IsNotExist(err) {
		t.Fatalf("busy image should prevent upload: %v", err)
	}
}

func TestReleaseWorkflowRequiresCleanCheckoutForDraft(t *testing.T) {
	fixture := newReleaseWorkflowFixture(t)
	if err := os.WriteFile(filepath.Join(fixture.repositoryDirectory, "unfinished.txt"), []byte("unfinished"), 0600); err != nil {
		t.Fatal(err)
	}
	output, err := fixture.run("draft")
	if err == nil || !strings.Contains(output, "Commit and merge your changes") {
		t.Fatalf("dirty checkout should stop the draft: %v: %s", err, output)
	}
	if _, err := os.Stat(fixture.draftArgumentsPath); !os.IsNotExist(err) {
		t.Fatalf("draft upload should not have run: %v", err)
	}
	if output, err := fixture.run("check"); err != nil {
		t.Fatalf("local verification should allow work in progress: %v: %s", err, output)
	}
}

type releaseWorkflowFixture struct {
	repositoryDirectory string
	releaseDirectory    string
	temporaryDirectory  string
	commandLogPath      string
	draftArgumentsPath  string
	diskStateDirectory  string
	environment         []string
}

func newReleaseWorkflowFixture(t *testing.T) releaseWorkflowFixture {
	t.Helper()
	fixtureDirectory := t.TempDir()
	toolDirectory := filepath.Join(fixtureDirectory, "tools")
	fixture := releaseWorkflowFixture{
		repositoryDirectory: filepath.Join(fixtureDirectory, "checkout with spaces"),
		releaseDirectory:    filepath.Join(fixtureDirectory, "release with spaces"),
		temporaryDirectory:  filepath.Join(fixtureDirectory, "temporary files"),
		commandLogPath:      filepath.Join(fixtureDirectory, "commands.log"),
		draftArgumentsPath:  filepath.Join(fixtureDirectory, "draft-arguments"),
		diskStateDirectory:  filepath.Join(fixtureDirectory, "disk state"),
	}
	for _, directory := range []string{toolDirectory, fixture.temporaryDirectory, fixture.diskStateDirectory, fixture.releaseDirectory} {
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(fixture.releaseDirectory, "Kaffeinate-macOS-universal.zip"), []byte("previous release"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeDiskImageFakes(t, toolDirectory)
	fixtureFiles := map[string]string{
		"scripts/release.sh": `#!/bin/bash
set -euo pipefail
echo build >> "$RELEASE_TEST_COMMAND_LOG"
if [[ ${RELEASE_TEST_RESULT:-} == build ]]; then echo 'build failed'; exit 1; fi
mkdir -p "$1"
bundle_directory="$DMG_TEST_STATE/payload"
mkdir -p "$bundle_directory/Kaffeinate.app/Contents" "$bundle_directory/.background"
cp packaging/macos/Info.plist "$bundle_directory/Kaffeinate.app/Contents/Info.plist"
cp packaging/macos/dmg-background.png "$bundle_directory/.background/background.png"
ln -sfn /Applications "$bundle_directory/Applications"
printf layout > "$bundle_directory/.DS_Store"
printf image > "$1/Kaffeinate-macOS-universal.dmg"
cd "$1"
shasum -a 256 Kaffeinate-macOS-universal.dmg > SHA256SUMS.txt
if [[ ${RELEASE_TEST_RESULT:-} == checksum ]]; then printf invalid > SHA256SUMS.txt; fi
`,
		"packaging/macos/Info.plist": `<plist version="1.0"><dict>
<key>CFBundleShortVersionString</key><string>1.2.3</string>
<key>CFBundleVersion</key><string>4</string>
</dict></plist>
`,
	}
	backgroundBytes, err := os.ReadFile("../packaging/macos/dmg-background.png")
	if err != nil {
		t.Fatal(err)
	}
	fixtureFiles["packaging/macos/dmg-background.png"] = string(backgroundBytes)
	for _, scriptName := range []string{"release-workflow.sh", "copy-dmg-app.sh", "disk-image.sh", "test.sh", "vet.sh"} {
		scriptBytes, err := os.ReadFile(filepath.Join("..", "scripts", scriptName))
		if err != nil {
			t.Fatal(err)
		}
		fixtureFiles[filepath.Join("scripts", scriptName)] = string(scriptBytes)
	}
	for relativePath, fileContents := range fixtureFiles {
		filePath := filepath.Join(fixture.repositoryDirectory, relativePath)
		if err := os.MkdirAll(filepath.Dir(filePath), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filePath, []byte(fileContents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for toolName, toolScript := range map[string]string{
		"pgrep": `case ${RELEASE_TEST_PROCESS:-} in
running) echo 4242; exit 0 ;;
error) exit 3 ;;
*) exit 1 ;;
esac
`,
		"go": `echo "go $*" >> "$RELEASE_TEST_COMMAND_LOG"
if [[ $1 == vet ]]; then
    if [[ ${RELEASE_TEST_RESULT:-} == vet ]]; then echo 'vet failed'; exit 1; fi
elif [[ $* == *./integration* ]]; then
    test -f "$KAFFEINATE_TEST_RELEASE_DIR/Kaffeinate-macOS-universal.dmg"
    test -f "$KAFFEINATE_TEST_APP_BUNDLE/Contents/Info.plist"
    test ! -f "$DMG_TEST_STATE/mounted"
    case ${RELEASE_TEST_RESULT:-} in
    integration) echo 'app checks failed'; exit 1 ;;
    skip) echo '--- SKIP: TestPackagedCLI (0.00s)'; exit 0 ;;
    architecture-skip) echo '    --- SKIP: TestReleaseAppArchitectures/x86_64 (0.00s)' ;;
    changed) echo changed > unfinished.txt ;;
    esac
    echo '--- PASS: TestPackagedCLI (0.01s)'
else
    test -z "${KAFFEINATE_TEST_RELEASE_DIR:-}"
    test -z "${KAFFEINATE_TEST_APP_BUNDLE:-}"
    if [[ ${RELEASE_TEST_RESULT:-} == unit ]]; then echo 'unit tests failed'; exit 1; fi
fi
`,
		"gh": `echo github >> "$RELEASE_TEST_COMMAND_LOG"
case "$1 $2" in
'repo view') echo release-tests/kaffeinate ;;
'api repos/release-tests/kaffeinate/commits/main')
    if [[ ${RELEASE_TEST_REMOTE:-} == unmerged ]]; then echo different-commit; else git rev-parse HEAD; fi ;;
'api --paginate')
    case ${RELEASE_TEST_REMOTE:-} in
    error) echo 'GitHub unavailable' >&2; exit 1 ;;
    release) echo v1.2.3 ;;
    esac ;;
'api repos/release-tests/kaffeinate/git/matching-refs/tags/v1.2.3')
    if [[ ${RELEASE_TEST_REMOTE:-} == tag ]]; then echo refs/tags/v1.2.3; fi ;;
'release create')
    test -f "$4"
    test -f "$5"
    if [[ ${RELEASE_TEST_RESULT:-} == upload ]]; then echo 'upload failed'; exit 1; fi
    printf '%s\0' "$@" > "$RELEASE_TEST_DRAFT_ARGUMENTS" ;;
*) echo "unexpected GitHub command: $*" >&2; exit 1 ;;
esac
`,
	} {
		if err := os.WriteFile(filepath.Join(toolDirectory, toolName), []byte("#!/bin/bash\nset -euo pipefail\n"+toolScript), 0700); err != nil {
			t.Fatal(err)
		}
	}
	fixture.environment = append(os.Environ(),
		"PATH="+toolDirectory+":/usr/bin:/bin:/usr/sbin:/sbin",
		"TMPDIR="+fixture.temporaryDirectory,
		"RELEASE_TEST_COMMAND_LOG="+fixture.commandLogPath,
		"RELEASE_TEST_DRAFT_ARGUMENTS="+fixture.draftArgumentsPath,
		"DMG_TEST_STATE="+fixture.diskStateDirectory,
	)
	fixture.git(t, "init", "--quiet", "--initial-branch=main")
	fixture.git(t, "add", ".")
	fixture.git(t, "-c", "user.name=Release Test", "-c", "user.email=release@example.invalid",
		"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "--quiet", "-m", "Release fixture")
	return fixture
}

func (fixture releaseWorkflowFixture) git(t *testing.T, arguments ...string) string {
	t.Helper()
	command := exec.Command("/usr/bin/git", arguments...)
	command.Dir = fixture.repositoryDirectory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
	return strings.TrimSpace(string(output))
}

func (fixture releaseWorkflowFixture) run(action string, environment ...string) (string, error) {
	command := exec.Command("/bin/bash", filepath.Join(fixture.repositoryDirectory, "scripts", "release-workflow.sh"), action, fixture.releaseDirectory)
	command.Dir = fixture.temporaryDirectory
	command.Env = append(slices.Clone(fixture.environment), environment...)
	output, err := command.CombinedOutput()
	return string(output), err
}

func (fixture releaseWorkflowFixture) requireTemporaryCleanup(t *testing.T) {
	t.Helper()
	entries, err := os.ReadDir(fixture.temporaryDirectory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "kaffeinate release.") || strings.HasPrefix(entry.Name(), "kaffeinate image.") {
			t.Errorf("temporary release files were not cleaned up: %s", entry.Name())
		}
	}
	if _, err := os.Stat(filepath.Join(fixture.diskStateDirectory, "mounted")); !os.IsNotExist(err) {
		t.Fatalf("verification image remains attached: %v", err)
	}
}
