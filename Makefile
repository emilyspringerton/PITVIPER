BINARY := pitviper
CMD    := ./cmd/pitviper

.PHONY: build run install deps test

build:
	CGO_ENABLED=1 go build -o $(BINARY) $(CMD)

run: build
	./$(BINARY)

install:
	CGO_ENABLED=1 go install $(CMD)

test:
	go test ./internal/...

# Install system SDL2 dependencies (Ubuntu/Debian — requires sudo).
deps:
	sudo apt-get install -y libsdl2-dev
