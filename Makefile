APP_BUNDLE := dist/Kaffeinate.app
RELEASE_DIR ?= dist
INSTALL_DIR ?= $(HOME)/Applications
BIN_DIR ?= $(HOME)/.local/bin

.PHONY: build release release-check release-draft install test vet format

# Build the app bundle for the current Mac architecture.
build:
	bash scripts/build.sh "$(APP_BUNDLE)"

# Package the app and CLI for Apple Silicon and Intel Macs.
release:
	bash scripts/release.sh "$(RELEASE_DIR)"

# Build and verify the disk image and installed app.
release-check:
	bash scripts/release-workflow.sh check "$(RELEASE_DIR)"

# Build, verify, and upload a draft release to GitHub.
release-draft:
	bash scripts/release-workflow.sh draft "$(RELEASE_DIR)"

# Build and install the app, then link its CLI.
install: build
	bash scripts/install.sh "$(APP_BUNDLE)" "$(INSTALL_DIR)" "$(BIN_DIR)"

# Run the Go tests with the race detector enabled.
test:
	bash scripts/test.sh

# Check the Go code for likely mistakes.
vet:
	bash scripts/vet.sh

# Format Go and shell source files throughout the repository.
format:
	bash scripts/format.sh
