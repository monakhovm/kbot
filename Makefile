APP := $(shell basename $(shell git remote get-url origin) | cut -d. -f1)
REGISTRY = ghcr.io/monakhovm
VERSION := $(shell git describe --tags --abbrev=0)-$(shell git rev-parse --short HEAD)
SHORT_VERSION := $(shell git describe --tags --abbrev=0)
HELM_VERSION ?= $(shell cat helm/Chart.yaml | grep version | grep -oP '\d+\.\d+\.\d+')
TARGETARCH ?= amd64
TARGETOS ?= linux

format:
	gofmt -s -w ./

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
	docker build -t ${REGISTRY}/${APP}:${VERSION}-${TARGETOS}-$(TARGETARCH) .

push:
	docker push ${REGISTRY}/${APP}:${VERSION}-${TARGETOS}-$(TARGETARCH)
	yq -i '.image.tag = "${VERSION}"' ./helm/values.yaml

helmchart:
	helm package --version ${HELM_VERSION} --app-version ${HELM_VERSION}--destination ./helm helm
	helm push ./helm/kbot-${HELM_VERSION}.tgz oci://${REGISTRY}/charts

clean:
	rm -rf kbot
	docker rmi ${REGISTRY}/${APP}:${VERSION}-${TARGETOS}-$(TARGETARCH)
