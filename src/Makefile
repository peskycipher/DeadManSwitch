# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) install
BINARY_NAME=dmswitch
PLATFORMS=linux/amd64 linux/arm64 linux/386 windows/amd64 windows/arm64 darwin/amd64 darwin/arm64 freebsd/amd64

all: test build

build:
	$(GOBUILD) -o $(BINARY_NAME) -v .

test:
	$(GOTEST) -v .

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -rf release

run:
	$(GOCMD) run -v .

deps:
	$(GOGET) ./...

# cross-compile release artifacts to release/$GOOS/$GOARCH/
release:
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; arch=$${platform#*/}; \
		dir=release/$$os/$$arch; \
		mkdir -p $$dir; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 $(GOBUILD) -o $$dir/$(BINARY_NAME) -v . || exit 1; \
	done
	@ls -R release