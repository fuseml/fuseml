# Tekton operator version
TEKTON_OPERATOR_VERSION=v0.49.0

GOOS:=$(shell go env GOOS)
GOARCH:=$(shell go env GOARCH)

LDFLAGS:= -w -s

PKG_PATH=github.com/fuseml/fuseml/cli/paas

GIT_COMMIT = $(shell git rev-parse --short=8 HEAD)
GIT_BRANCH = $(shell git rev-parse --abbrev-ref HEAD|grep -v HEAD)
GIT_TAG    = $(shell git describe --tags --abbrev=0 --exact-match 2>/dev/null)
BUILD_DATE = $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

ifdef VERSION
	BINARY_VERSION = $(VERSION)
else
# Use `dev` as a default version value when compiling in the main branch
ifeq ($(GIT_BRANCH),main)
	BINARY_VERSION = dev
# Use the branch name as a default version value when compiling in another branch
else
	BINARY_VERSION = $(GIT_BRANCH)
endif
ifneq ($(GIT_TAG),)
	BINARY_VERSION = $(GIT_TAG)
else
endif
endif

LDFLAGS += -X $(PKG_PATH)/version.GitCommit=$(GIT_COMMIT)
LDFLAGS += -X $(PKG_PATH)/version.BuildDate=$(BUILD_DATE)
ifneq ($(BINARY_VERSION),)
LDFLAGS += -X $(PKG_PATH)/version.Version=$(BINARY_VERSION)
endif

########################################################################
## Development

# Embed files, run linter and build FuseML installer binary
build: embed_files lint build-installer

build-installer:
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags '$(LDFLAGS)' -o dist/fuseml-installer

test: embed_files
	ginkgo ./cmd/internal/client/ ./tools/ ./helpers/ ./kubernetes/

test-acceptance-traefik: embed_files
	@./scripts/test-acceptance.sh -- -serve=deployment

test-acceptance-knative: embed_files
	@./scripts/test-acceptance.sh -- -serve=knative

test-acceptance-kfserving: embed_files
	@./scripts/test-acceptance.sh -- -serve=kfserving

test-acceptance-seldon_mlflow: embed_files
	@./scripts/test-acceptance.sh -- -serve=seldon_mlflow

test-acceptance-seldon_sklearn: embed_files
	@./scripts/test-acceptance.sh -- -serve=seldon_sklearn

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
	helm package ./assets/trow/ -d embedded-files

update_charts: update_registry

update_tekton:
	mkdir -p embedded-files/tekton/install
	wget https://github.com/tektoncd/operator/releases/download/${TEKTON_OPERATOR_VERSION}/release.yaml -O embedded-files/tekton/install/operator.yaml
	wget https://raw.githubusercontent.com/tektoncd/operator/${TEKTON_OPERATOR_VERSION}/config/crs/kubernetes/config/all/operator_v1alpha1_config_cr.yaml -O embedded-files/tekton/install/profile-all.yaml

embed_files: tools
	statik -m -f -src=./embedded-files

help:
	( echo _ _ ___ _____ ________ Overview ; fuseml help ; for cmd in completion help info install uninstall ; do echo ; echo _ _ ___ _____ ________ Command $$cmd ; fuseml $$cmd --help ; done ; echo ) | tee HELP

########################################################################
## Release

# Embed files, run linter and build release-ready archived binaries for all supported ARCHs and OSs
release: embed_files lint release-all

release-installer: build-installer
	tar zcf dist/fuseml-installer-$(GOOS)-$(GOARCH).tar.gz -C dist/ --remove-files --transform="s#\.\/##" ./fuseml-installer
	cd dist && sha256sum -b fuseml-installer-$(GOOS)-$(GOARCH).tar.gz > fuseml-installer-$(GOOS)-$(GOARCH).tar.gz.sha256

release-all: release-amd64 release-arm64 release-arm32 release-darwin-amd64 release-darwin-arm64 release-windows

release-arm32:
	$(MAKE) GOARCH="arm" GOOS="linux" release-installer

release-arm64:
	$(MAKE) GOARCH="arm64" GOOS="linux" release-installer

release-amd64:
	$(MAKE) GOARCH="amd64" GOOS="linux" release-installer

release-darwin-amd64:
	$(MAKE) GOARCH="amd64" GOOS="darwin" release-installer

release-darwin-arm64:
	$(MAKE) GOARCH="arm64" GOOS="darwin" release-installer

release-windows:
	$(MAKE) GOARCH="amd64" GOOS="windows" release-installer

########################################################################
# Support

tools-install:
	@./scripts/tools-install.sh

tools-versions:
	@./scripts/tools-versions.sh

istio-install:
	@./scripts/istio-minimal-install.sh

seldon-install:
	@./scripts/seldon-operator-install.sh

k3d-install:
	export TAG=v4.4.7 && curl -s https://raw.githubusercontent.com/rancher/k3d/main/install.sh | bash

new-test-cluster:
	@./scripts/ci/k3d-cluster.sh create

delete-test-cluster:
	@./scripts/ci/k3d-cluster.sh delete

mlflow-e2e:
	@./scripts/ci/mlflow-e2e.sh

fuseml-install:
	@./dist/fuseml-installer install

fuseml-install-with-extensions:
	@./dist/fuseml-installer install --extensions mlflow,kfserving

test-mlflow-e2e: build new-test-cluster fuseml-install-with-extensions mlflow-e2e delete-test-cluster


########################################################################
# Kube dev environments

minikube-start:
	@./scripts/minikube-start.sh

minikube-delete:
	@./scripts/minikube-delete.sh
