
########################################################################
## Development

build: embed_files lint build-local

build-all: embed_files lint build-amd64 build-arm64 build-arm32 build-windows build-darwin

build-all-small:
	@$(MAKE) LDFLAGS+="-s -w" build-all

build-local: lint
	go build -ldflags '$(LDFLAGS)' -o dist/fuseml

build-arm32: lint
	GOARCH="arm" GOOS="linux" go build -ldflags '$(LDFLAGS)' -o dist/fuseml-linux-arm32

build-arm64: lint
	GOARCH="arm64" GOOS="linux" go build -ldflags '$(LDFLAGS)' -o dist/fuseml-linux-arm64

build-amd64: lint
	GOARCH="amd64" GOOS="linux" go build -race -ldflags '$(LDFLAGS)' -o dist/fuseml-linux-amd64

build-windows: lint
	GOARCH="amd64" GOOS="windows" go build -ldflags '$(LDFLAGS)' -o dist/fuseml-windows-amd64

build-darwin: lint
	GOARCH="amd64" GOOS="darwin" go build -ldflags '$(LDFLAGS)' -o dist/fuseml-darwin-amd64

build-darwin-arm64: lint
	GOARCH="arm64" GOOS="darwin" go build -ldflags '$(LDFLAGS)' -o dist/fuseml-darwin-arm64

compress:
	upx --brute -1 ./dist/fuseml-linux-arm32
	upx --brute -1 ./dist/fuseml-linux-arm64
	upx --brute -1 ./dist/fuseml-linux-amd64
	upx --brute -1 ./dist/fuseml-windows-amd64
	upx --brute -1 ./dist/fuseml-darwin-amd64

test: embed_files
	ginkgo ./cmd/internal/client/ ./tools/ ./helpers/ ./kubernetes/

test-acceptance-traefik: embed_files
	@./scripts/test-acceptance.sh -- -serve=deployment

test-acceptance-knative: embed_files
	@./scripts/test-acceptance.sh -- -serve=knative

test-acceptance-kfserving: embed_files
	@./scripts/test-acceptance.sh -- -serve=kfserving

generate:
	go generate ./...

lint:	embed_files fmt vet tidy

vet:
	go vet ./...

tidy:
	go mod tidy

fmt:
	go fmt ./...

gitlint:
	gitlint --commits "origin..HEAD"

.PHONY: tools
tools:
	go get github.com/rakyll/statik


update_registry:
	helm package ./assets/container-registry/chart/container-registry/ -d embedded-files

update_mlflow:
	helm package ./assets/mlflow/ -d embedded-files

update_charts: update_registry update_mlflow

update_tekton:
	mkdir -p embedded-files/tekton
	wget https://storage.googleapis.com/tekton-releases/pipeline/previous/v0.19.0/release.yaml -O embedded-files/tekton/pipeline-v0.19.0.yaml
	wget https://storage.googleapis.com/tekton-releases/triggers/previous/v0.11.1/release.yaml -O embedded-files/tekton/triggers-v0.11.1.yaml
	wget https://github.com/tektoncd/dashboard/releases/download/v0.11.1/tekton-dashboard-release.yaml -O embedded-files/tekton/dashboard-v0.11.1.yaml

embed_files: tools
	statik -m -f -src=./embedded-files

help:
	( echo _ _ ___ _____ ________ Overview ; fuseml help ; for cmd in apps completion create-org delete help info install orgs push target uninstall ; do echo ; echo _ _ ___ _____ ________ Command $$cmd ; fuseml $$cmd --help ; done ; echo ) | tee HELP

########################################################################
# Support

tools-install:
	@./scripts/tools-install.sh

tools-versions:
	@./scripts/tools-versions.sh

istio-install:
	@./scripts/istio-minimal-install.sh

knative-install: istio-install
	@./scripts/knative-install.sh

cert-manager-install:
	@./scripts/cert-manager-install.sh

kfserving-install: knative-install cert-manager-install
	@./scripts/kfserving-install.sh

########################################################################
# Kube dev environments

minikube-start:
	@./scripts/minikube-start.sh

minikube-delete:
	@./scripts/minikube-delete.sh
