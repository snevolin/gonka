.PHONY: generate generate-proto build test check-gen

generate: generate-proto

generate-proto:
	go generate ./nodemanager/...

build:
	go build ./...

test:
	go test ./... -v

check-gen: generate
	git diff --exit-code nodemanager/gen/
