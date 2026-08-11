VERSION := 1.0.0
BINARY := snap
DIST := dist

.PHONY: all clean build-all build-mac build-mac-arm build-windows build-linux

all: build-all

clean:
	rm -rf $(DIST)

build-all: clean build-mac build-mac-arm build-windows build-linux

build-mac:
	@mkdir -p $(DIST)
	GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o $(DIST)/$(BINARY)-darwin-amd64 ./cmd/snap/

build-mac-arm:
	@mkdir -p $(DIST)
	GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o $(DIST)/$(BINARY)-darwin-arm64 ./cmd/snap/

build-windows:
	@mkdir -p $(DIST)
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o $(DIST)/$(BINARY)-windows-amd64.exe ./cmd/snap/

build-linux:
	@mkdir -p $(DIST)
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(DIST)/$(BINARY)-linux-amd64 ./cmd/snap/

install: build-mac-arm
	cp $(DIST)/$(BINARY)-darwin-arm64 /usr/local/bin/$(BINARY)
