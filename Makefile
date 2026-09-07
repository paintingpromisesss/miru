BINARY=miru
LDFLAGS=-s -w -buildid=
BUILD_CMD=CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)"

.PHONY: all build build-linux-arm64 build-linux-mipsle build-linux-mips build-linux-armv7 build-linux-amd64 clean test

all: build

build:
	$(BUILD_CMD) -o $(BINARY) ./cmd/miru

build-linux-arm64:
	GOOS=linux GOARCH=arm64 $(BUILD_CMD) -o $(BINARY)-linux-arm64 ./cmd/miru

# MT7621 / MT7628 lack hardware FPU
build-linux-mipsle:
	GOOS=linux GOARCH=mipsle GOMIPS=softfloat $(BUILD_CMD) -o $(BINARY)-linux-mipsle ./cmd/miru

build-linux-mips:
	GOOS=linux GOARCH=mips GOMIPS=softfloat $(BUILD_CMD) -o $(BINARY)-linux-mips ./cmd/miru

build-linux-armv7:
	GOOS=linux GOARCH=arm GOARM=7 $(BUILD_CMD) -o $(BINARY)-linux-armv7 ./cmd/miru

build-linux-amd64:
	GOOS=linux GOARCH=amd64 $(BUILD_CMD) -o $(BINARY)-linux-amd64 ./cmd/miru

test:
	go test -v ./...

clean:
	@rm -f $(BINARY) $(BINARY)-*
