PROJECT_NAME := Pulumi Dokploy Provider
PACK := dokploy
PROJECT := github.com/dimeskigj/pulumi-dokploy
PROVIDER := pulumi-resource-$(PACK)
PROVIDER_PATH := provider
VERSION_GENERIC ?= $(or $(PROVIDER_VERSION),0.0.1-alpha.0+dev)
# Maven Central currently fails TLS 1.3 session resumption on the CI Java/Maven
# combination. Keep certificate validation enabled while using TLS 1.2.
MAVEN_TLS_OPTS := -Djdk.tls.client.protocols=TLSv1.2 -Dhttps.protocols=TLSv1.2
JAVA11_EXEC := mise exec java@temurin-11.0.32+101 gradle@8.14.3 --
JAVA17_MAVEN_EXEC := mise exec java@temurin-17.0.20+8 maven@3.9.11 --

.PHONY: provider provider_no_deps codegen generate_schema generate_go generate_nodejs generate_python generate_dotnet generate_java build_go build_python build_nodejs build_dotnet build_java build_sdks install_go_sdk install_python_sdk install_nodejs_sdk install_dotnet_sdk install_java_sdk install_plugin gen_examples test_examples test test_provider test_race check_codegen govulncheck license lint generate_openapi check_openapi build prepare_local_workspace local_generate docs_generate docs_check docs_build

provider:
	mkdir -p bin
	go build -ldflags "-X $(PROJECT)/provider.Version=$(VERSION_GENERIC)" -o bin/$(PROVIDER) ./provider/cmd/pulumi-resource-$(PACK)

provider_no_deps: provider

prepare_local_workspace:

generate_schema: provider
	mise exec pulumi@3.259.0 -- pulumi package get-schema $(CURDIR)/bin/$(PROVIDER) > provider/cmd/$(PROVIDER)/schema.json
	python3 scripts/normalize-generated.py schema provider/cmd/$(PROVIDER)/schema.json

generate_go generate_nodejs generate_python generate_dotnet generate_java: codegen

local_generate: codegen

codegen: provider
	mkdir -p provider/cmd/$(PROVIDER) sdk
	mise exec pulumi@3.259.0 -- pulumi package get-schema $(CURDIR)/bin/$(PROVIDER) > provider/cmd/$(PROVIDER)/schema.json
	rm -rf sdk/nodejs sdk/python sdk/go sdk/dotnet sdk/java
	mise exec pulumi@3.259.0 -- pulumi package gen-sdk provider/cmd/$(PROVIDER)/schema.json --language all -o sdk
	python3 scripts/remove-dotnet-package-icon.py sdk/dotnet/Dimeskigj.Pulumi.Dokploy.csproj
	rm -f sdk/dotnet/logo.png
	printf '%s' '$(VERSION_GENERIC)' > sdk/dotnet/version.txt
	cp go.mod sdk/go/$(PACK)/go.mod
	cd sdk/go/$(PACK) && mise exec -- go mod edit -module=$(PROJECT)/sdk/go/$(PACK) -dropreplace=$(PROJECT)/sdk/go/$(PACK)
	cd sdk/go/$(PACK) && mise exec -- go mod tidy
	python3 scripts/normalize-generated.py sdk sdk
	python3 scripts/normalize-generated.py schema provider/cmd/$(PROVIDER)/schema.json

build_go:
	cd sdk/go/$(PACK) && mise exec -- go test ./...

build_python:
	rm -rf sdk/python/bin sdk/python-tmp
	mkdir sdk/python-tmp
	cp -R sdk/python/. sdk/python-tmp/
	mv sdk/python-tmp sdk/python/bin
	sed -i.bak -e 's/^VERSION = ".*"/VERSION = "$(VERSION_GENERIC)"/' sdk/python/bin/setup.py
	rm sdk/python/bin/setup.py.bak
	cd sdk/python/bin && python3 -m venv venv && ./venv/bin/python -m pip install --quiet build && ./venv/bin/python -m build .

build_nodejs:
	cd sdk/nodejs && npm install --package-lock=false --ignore-scripts --no-audit --no-fund && npm run build
	cp sdk/nodejs/README.md LICENSE sdk/nodejs/package.json sdk/nodejs/bin/
	sed -i.bak -e 's/$${VERSION}/$(VERSION_GENERIC)/g' sdk/nodejs/bin/package.json
	rm sdk/nodejs/bin/package.json.bak

build_dotnet:
	printf '%s' '$(VERSION_GENERIC)' > sdk/dotnet/version.txt
	cd sdk/dotnet && dotnet build --nologo -p:Version=$(VERSION_GENERIC)

build_java:
	sed -i.bak -e 's/^                name = ""/                name = "$(PROJECT_NAME)"/' sdk/java/build.gradle
	rm sdk/java/build.gradle.bak
	cd sdk/java && gradle build --no-daemon

install_go_sdk:
	cd examples/go && go mod tidy

install_python_sdk:
	python3 -m compileall -q examples/python

install_nodejs_sdk:
	cd sdk/nodejs && npm install --package-lock=false --ignore-scripts --no-audit --no-fund
	cd examples/nodejs && npm install --package-lock=false --ignore-scripts --no-audit --no-fund && npx tsc --noEmit

install_dotnet_sdk:
	cd examples/dotnet && dotnet build --nologo

install_java_sdk:
	$(eval JAVA_EXAMPLE_VERSION := $(shell grep -A1 '<artifactId>dokploy</artifactId>' examples/java/pom.xml | grep -oE '<version>[^<]*</version>' | sed -E 's/<\/?version>//g'))
	cd sdk/java && PACKAGE_VERSION=$(JAVA_EXAMPLE_VERSION) $(JAVA11_EXEC) gradle publishToMavenLocal --no-daemon
	cd examples/java && $(JAVA17_MAVEN_EXEC) mvn $(MAVEN_TLS_OPTS) package -DskipTests

build_sdks: build_go build_python build_nodejs build_dotnet build_java

install_plugin: provider
	@if [ -n "$(PULUMI_HOME)" ]; then PULUMI_HOME="$(PULUMI_HOME)" mise exec pulumi@3.259.0 -- pulumi plugin install resource dokploy $(VERSION_GENERIC) --file bin/$(PROVIDER) --reinstall; else mise exec pulumi@3.259.0 -- pulumi plugin install resource dokploy $(VERSION_GENERIC) --file bin/$(PROVIDER) --reinstall; fi

gen_examples: codegen
	scripts/gen-examples.sh "$(MAKE)" "$(CURDIR)" "$(PROJECT)" "$(PACK)" "$(VERSION_GENERIC)"

test_examples: install_plugin
	mise exec -- go test ./examples -tags=all -count=1
	cd examples/go && go test . -count=1
	python3 -m compileall -q examples/python
	cd sdk/nodejs && npm install --package-lock=false --ignore-scripts --no-audit --no-fund
	cd examples/nodejs && npm install --package-lock=false --ignore-scripts --no-audit --no-fund && npx tsc --noEmit
	cd examples/dotnet && dotnet build --nologo
	cd sdk/java && $(JAVA11_EXEC) gradle publishToMavenLocal --no-daemon
	cd examples/java && $(JAVA17_MAVEN_EXEC) mvn $(MAVEN_TLS_OPTS) package -DskipTests

test_provider:
	go test -short -v -count=1 ./provider/... ./internal/...

test_race:
	mise exec -- go test -race ./provider/... ./internal/...

test: test_provider test_examples

lint:
	golangci-lint run

generate_openapi:
	mise exec -- go run ./openapi/cmd/normalize -in openapi/upstream.json -out openapi/dokploy.json
	mise exec -- go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 --config openapi/oapi-codegen.yaml openapi/dokploy.json
	mise exec -- gofmt -w internal/client/generated/generated.gen.go openapi/cmd/normalize/main.go openapi/cmd/normalize/main_test.go

check_openapi: generate_openapi
	git diff --exit-code -- openapi/dokploy.json internal/client/generated/generated.gen.go

check_codegen:
	python3 -m unittest discover -s scripts -p 'test_normalize_generated.py'
	$(MAKE) VERSION_GENERIC=0.0.1-alpha.0+dev codegen
	python3 scripts/check-schema-drift.py provider/cmd/$(PROVIDER)/schema.json
	git diff --exit-code -- sdk

govulncheck:
	mise exec -- go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...

license:
	mise exec -- go run github.com/google/go-licenses@v1.6.0 check ./...

build: provider

docs_generate:
	npm ci --prefix website
	npm --prefix website run generate

docs_check:
	npm ci --prefix website
	npm --prefix website run check:generated
	npm --prefix website run check
	npm --prefix website run build
	npm --prefix website run test:built

docs_build:
	npm ci --prefix website
	npm --prefix website run build
