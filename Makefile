APP_BUNDLE := dist/Kaffeinate.app
INSTALL_DIR ?= $(HOME)/Applications
BIN_DIR ?= $(HOME)/.local/bin

.PHONY: build install test vet

build:
	@test "$$(uname -s)" = Darwin || { echo "The application currently builds on macOS."; exit 1; }
	rm -rf "$(APP_BUNDLE)"
	mkdir -p "$(APP_BUNDLE)/Contents/MacOS" "$(APP_BUNDLE)/Contents/Resources/bin"
	CGO_ENABLED=1 go build -trimpath -o "$(APP_BUNDLE)/Contents/MacOS/Kaffeinate" ./cmd/kaffeinate-app
	CGO_ENABLED=0 go build -trimpath -o "$(APP_BUNDLE)/Contents/Resources/bin/kaffeinate" ./cmd/kaffeinate
	cp packaging/macos/Info.plist "$(APP_BUNDLE)/Contents/Info.plist"
	go run ./tools/icons "$(APP_BUNDLE)/Contents/Resources/Kaffeinate.icns"
	plutil -lint "$(APP_BUNDLE)/Contents/Info.plist"

install: build
	@case "$(INSTALL_DIR)" in /*) ;; *) echo "INSTALL_DIR must be an absolute path."; exit 1;; esac
	@case "$(BIN_DIR)" in /*) ;; *) echo "BIN_DIR must be an absolute path."; exit 1;; esac
	@set -e; \
	target="$(INSTALL_DIR)/Kaffeinate.app"; \
	if [ -e "$$target" ] || [ -L "$$target" ]; then \
		test ! -L "$$target" && test "$$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$$target/Contents/Info.plist")" = local.kaffeinate.app || { echo "The destination is not a Kaffeinate application bundle."; exit 1; }; \
	fi; \
	if [ -e "$(BIN_DIR)/kaffeinate" ] && [ ! -L "$(BIN_DIR)/kaffeinate" ]; then echo "A non-symlink kaffeinate command already exists in BIN_DIR."; exit 1; fi; \
	mkdir -p "$(INSTALL_DIR)" "$(BIN_DIR)"; \
	staging="$$(mktemp -d "$(INSTALL_DIR)/.kaffeinate-install.XXXXXX")"; \
	trap 'rm -rf "$$staging"' EXIT; \
	cp -R "$(APP_BUNDLE)" "$$staging/Kaffeinate.app"; \
	rm -rf "$$target"; \
	mv "$$staging/Kaffeinate.app" "$$target"; \
	ln -sfn "$$target/Contents/Resources/bin/kaffeinate" "$(BIN_DIR)/kaffeinate"; \
	echo "Installed $$target and $(BIN_DIR)/kaffeinate"

test:
	go test -race ./...

vet:
	go vet ./...
