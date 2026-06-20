.PHONY: build test tidy generate generate-api check-gen

VERSION ?= $(shell git describe --always)
BLST_PORTABLE ?= 0

BUILD_FLAGS := -ldflags "-X main.version=$(VERSION)"

ifeq ($(BLST_PORTABLE),1)
export CGO_CFLAGS := $(CGO_CFLAGS) -D__BLST_PORTABLE__
endif

generate: generate-api

generate-api:
	go generate ./queryapi/...

check-gen: generate-api
	git diff --exit-code queryapi/gen/

build:
	CGO_ENABLED=1 go build $(BUILD_FLAGS) -o build/edge-api ./cmd/edge-api

test:
	go test ./... -count=1

tidy:
	go mod tidy
