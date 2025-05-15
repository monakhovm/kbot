APP := $(shell basename $(shell git remote get-url origin) | cut -d. -f1)
REGISTRY := ghcr.io/monakhovm
VERSION := $(shell git describe --tags --abbrev=0)-$(shell git rev-parse --short HEAD)
TARGETOS := linux #darwin windows linux
TARGETARCH := arm64 #amd64 386 arm arm64

format:
	gofmt -s -w ./

lint:
	golint

test:
	go test -v

get:
	go get

arm:
	$(MAKE) build TARGETARCH=arm64 TARGETOS=linux

linux:
	$(MAKE) build TARGETARCH=amd64 TARGETOS=linux

macos:
	$(MAKE) build TARGETARCH=arm64 TARGETOS=darwin

windows:
	$(MAKE) build TARGETARCH=amd64 TARGETOS=windows

build: format get
	CGO_ENABLED=0 GOOS=$(TARGETOS) GOARCH=$(TARGETARCH) go build -v -o kbot -ldflags "-X="github.com/monakhovm/kbot/cmd.appVersion=${VERSION}

image:
	podman build --platform linux/amd64,linux/arm64 --env TARGETPLATFORM=$(TARGETOS) -t ${REGISTRY}/${APP}:${VERSION}-${TARGETARCH} .

push:
	podman push ${REGISTRY}/${APP}:${VERSION}-${TARGETARCH}

clean:
	rm -rf kbot
	podman rmi ${REGISTRY}/${APP}:${VERSION}-${TARGETARCH}
