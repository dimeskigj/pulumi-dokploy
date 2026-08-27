PROJECT_NAME := Pulumi Dokploy Provider
PACK := dokploy
PROJECT := github.com/gjorgjidimeski/pulumi-dokploy
PROVIDER := pulumi-resource-$(PACK)
PROVIDER_PATH := provider
VERSION_GENERIC ?= 0.0.1-alpha.0+dev

.PHONY: provider test test_provider lint

provider:
	go build -ldflags "-X $(PROJECT)/provider.Version=$(VERSION_GENERIC)" -o bin/$(PROVIDER) ./provider/cmd/pulumi-resource-$(PACK)

test_provider:
	go test -short -v -count=1 ./provider/... ./internal/...

test: test_provider

lint:
	golangci-lint run
