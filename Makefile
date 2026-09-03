PROJECT_NAME := Pulumi Dokploy Provider
PACK := dokploy
PROJECT := github.com/dimeskigj/pulumi-dokploy
PROVIDER := pulumi-resource-$(PACK)
PROVIDER_PATH := provider
VERSION_GENERIC ?= 0.0.1-alpha.0+dev

.PHONY: provider provider_no_deps codegen generate_schema generate_go generate_nodejs generate_python generate_dotnet generate_java build_go build_python build_nodejs build_dotnet build_java build_sdks install_go_sdk install_python_sdk install_nodejs_sdk install_dotnet_sdk install_java_sdk install_plugin gen_examples test_examples test test_provider test_race check_codegen govulncheck license lint generate_openapi check_openapi build prepare_local_workspace local_generate sign-goreleaser-exe-% docs_generate docs_check docs_build

provider:
	mkdir -p bin
	go build -ldflags "-X $(PROJECT)/provider.Version=$(VERSION_GENERIC)" -o bin/$(PROVIDER) ./provider/cmd/pulumi-resource-$(PACK)

provider_no_deps: provider

prepare_local_workspace:

generate_schema: provider
	mise exec pulumi@3.259.0 -- pulumi package get-schema $(CURDIR)/bin/$(PROVIDER) > provider/cmd/$(PROVIDER)/schema.json

generate_go generate_nodejs generate_python generate_dotnet generate_java: codegen

local_generate: codegen

sign-goreleaser-exe-%:
	@:

codegen: provider
	mkdir -p provider/cmd/$(PROVIDER) sdk
	mise exec pulumi@3.259.0 -- pulumi package get-schema $(CURDIR)/bin/$(PROVIDER) > provider/cmd/$(PROVIDER)/schema.json
	rm -rf sdk/nodejs sdk/python sdk/go sdk/dotnet sdk/java
	mise exec pulumi@3.259.0 -- pulumi package gen-sdk provider/cmd/$(PROVIDER)/schema.json --language all -o sdk
	printf '%s' '$(VERSION_GENERIC)' > sdk/dotnet/version.txt

build_go:
	mise exec -- go test ./sdk/go/...

build_python:
	python3 -m compileall -q sdk/python

build_nodejs:
	cd sdk/nodejs && npm install --package-lock=false --ignore-scripts --no-audit --no-fund && npm run build

build_dotnet:
	cd sdk/dotnet && dotnet build --nologo

build_java:
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
	cd sdk/java && gradle publishToMavenLocal --no-daemon
	cd examples/java && mvn package -DskipTests

build_sdks: build_go build_python build_nodejs build_dotnet build_java

install_plugin: provider
	mise exec pulumi@3.259.0 -- pulumi plugin install resource dokploy $(VERSION_GENERIC) --file bin/$(PROVIDER) --reinstall

gen_examples: codegen install_plugin
	rm -rf examples/nodejs examples/python examples/go examples/dotnet examples/java
	mise exec pulumi@3.259.0 -- pulumi convert --from yaml --language typescript --cwd examples/yaml --out ../nodejs --generate-only
	mise exec pulumi@3.259.0 -- pulumi convert --from yaml --language python --cwd examples/yaml --out ../python --generate-only
	mise exec pulumi@3.259.0 -- pulumi convert --from yaml --language go --cwd examples/yaml --out ../go --generate-only
	mise exec pulumi@3.259.0 -- pulumi convert --from yaml --language csharp --cwd examples/yaml --out ../dotnet --generate-only
	mise exec pulumi@3.259.0 -- pulumi convert --from yaml --language java --cwd examples/yaml --out ../java --generate-only
	go mod edit -require=$(PROJECT)@v0.0.0 -replace=$(PROJECT)=../../ examples/go/go.mod
	cd examples/go && go mod tidy
	python3 -c 'from pathlib import Path; p=Path("examples/go/go.mod"); s=p.read_text(); s=s.replace(" => " + str(Path.cwd()), " => ../../"); p.write_text(s)'
	cd examples/nodejs && npm pkg set dependencies.@dimeskigj/pulumi-dokploy=file:../../sdk/nodejs
	printf '%s\n' '-e ../../sdk/python' > examples/python/requirements.txt
	python3 -c 'from pathlib import Path; p=Path("examples/dotnet/dokploy-mvp.csproj"); p.write_text(p.read_text().replace('"'"'<PackageReference Include="Pulumi.Dokploy" Version="0.0.1-alpha.0+dev" />'"'"', '"'"'<ProjectReference Include="../../sdk/dotnet/Pulumi.Dokploy.csproj" />'"'"'))'
	python3 -c 'from pathlib import Path; p=Path("examples/java/pom.xml"); p.write_text(p.read_text().replace("<groupId>com.dimeskigj</groupId>", "<groupId>net.dimeski.pulumi</groupId>"))'
	python3 -c 'from pathlib import Path; p=Path("examples/java/pom.xml"); p.write_text(p.read_text().replace("<maven.compiler.source>11</maven.compiler.source>", "<maven.compiler.source>17</maven.compiler.source>").replace("<maven.compiler.target>11</maven.compiler.target>", "<maven.compiler.target>17</maven.compiler.target>").replace("<maven.compiler.release>11</maven.compiler.release>", "<maven.compiler.release>17</maven.compiler.release>"))'
	python3 -c 'import re; from pathlib import Path; p=Path("examples/java/src/main/java/generated_program/App.java"); s=p.read_text().replace("com.dimeskigj.dokploy", "net.dimeski.pulumi.dokploy").replace("config.requireObject(\"dokploy:endpoint\", com.pulumi.core.TypeShape.map(String.class, Object.class))", "config.require(\"dokploy:endpoint\")").replace("config.requireObject(\"dokploy:apiKey\", com.pulumi.core.TypeShape.map(String.class, Object.class))", "config.requireSecret(\"dokploy:apiKey\")"); s=re.sub(r'"'"'config\.getSecret\("(\w+)"\)\.orElse\("([^"]*)"\)'"'"', r'"'"'config.getSecret("\1").applyValue(v -> v.orElse("\2"))'"'"', s); p.write_text(s)'
	python3 -c 'import re; from pathlib import Path; p=Path("examples/dotnet/Program.cs"); s=re.sub(r'"'"'config\.GetSecret\("(\w+)"\) \?\? "([^"]*)"'"'"', r'"'"'config.GetSecret("\1") ?? Output.CreateSecret("\2")'"'"', p.read_text()); p.write_text(s)'
	python3 -c 'from pathlib import Path; [Path(x).write_text(chr(10).join(line.rstrip() for line in Path(x).read_text().splitlines()).rstrip()+chr(10)) for x in ["examples/dotnet/Program.cs", "examples/java/pom.xml"]]'
	python3 website/scripts/normalize-examples.py
	cp examples/yaml/README.md examples/nodejs/README.md
	cp examples/yaml/README.md examples/python/README.md
	cp examples/yaml/README.md examples/go/README.md
	cp examples/yaml/README.md examples/dotnet/README.md
	cp examples/yaml/README.md examples/java/README.md

test_examples: install_plugin
	mise exec -- go test ./examples -tags=all -count=1
	cd examples/go && go test . -count=1
	python3 -m compileall -q examples/python
	cd sdk/nodejs && npm install --package-lock=false --ignore-scripts --no-audit --no-fund
	cd examples/nodejs && npm install --package-lock=false --ignore-scripts --no-audit --no-fund && npx tsc --noEmit
	cd examples/dotnet && dotnet build --nologo
	cd sdk/java && gradle publishToMavenLocal --no-daemon
	cd examples/java && mvn package -DskipTests

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

check_codegen: codegen
	git diff --exit-code -- provider/cmd/$(PROVIDER)/schema.json sdk

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
