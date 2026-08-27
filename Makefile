PROJECT_NAME := Pulumi Dokploy Provider
PACK := dokploy
PROJECT := github.com/gjorgjidimeski/pulumi-dokploy
PROVIDER := pulumi-resource-$(PACK)
PROVIDER_PATH := provider
VERSION_GENERIC ?= 0.0.1-alpha.0+dev

.PHONY: provider test test_provider lint generate_openapi check_openapi

provider:
	mkdir -p bin
	go build -ldflags "-X $(PROJECT)/provider.Version=$(VERSION_GENERIC)" -o bin/$(PROVIDER) ./provider/cmd/pulumi-resource-$(PACK)

test_provider:
	go test -short -v -count=1 ./provider/... ./internal/...

test: test_provider

lint:
	golangci-lint run

generate_openapi:
	mise exec -- go run ./openapi/cmd/normalize -in openapi/upstream.json -out openapi/dokploy.json
	mise exec -- go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 --config openapi/oapi-codegen.yaml openapi/dokploy.json
	mise exec -- gofmt -w internal/client/generated/generated.gen.go openapi/cmd/normalize/main.go openapi/cmd/normalize/main_test.go

check_openapi: generate_openapi
	git diff --exit-code -- openapi/dokploy.json internal/client/generated/generated.gen.go
