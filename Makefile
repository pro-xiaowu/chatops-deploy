.DEFAULT_GOAL := help
.PHONY: help fmt vet test web-install web-build run docker
help:
	@echo "Targets: fmt vet test web-install web-build run docker"
fmt:
	gofmt -w $$(find . -name '*.go' -not -path './.tools/*')
vet:
	go vet ./...
test:
	go test ./... -race
web-install:
	npm --prefix web install
web-build:
	npm --prefix web run build
run:
	go run ./cmd/server
docker:
	docker build -t chatops-deploy:local .
