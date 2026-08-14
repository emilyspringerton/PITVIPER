BINARY := pitviper
CMD    := ./cmd/pitviper
INSTALL_PATH := $(HOME)/.local/bin/$(BINARY)

.PHONY: build run install deps test

build:
	GOWORK=off CGO_ENABLED=1 go build -o $(BINARY) $(CMD)

run: build
	./$(BINARY)

# Installs to ~/.local/bin, same convention as emily.cli/scripts/build.sh and
# IDUNA's CLI — that dir is already on PATH (see ~/.profile, ~/.bashrc), unlike
# `go install`'s default $GOPATH/bin which isn't. Makes `pitviper` runnable
# as a bare command from any shell, matching how SHANKPIT/GFD/REDGARDEN
# clients are handed to a user as a ready-to-run binary, not a source build.
install: build
	mkdir -p $(HOME)/.local/bin
	cp $(BINARY) $(INSTALL_PATH)
	@echo "installed -> $(INSTALL_PATH)"

test:
	GOWORK=off go test ./internal/...

# Install system SDL2 dependencies (Ubuntu/Debian — requires sudo).
deps:
	sudo apt-get install -y libsdl2-dev
