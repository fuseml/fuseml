FROM golang:1.16 as builder

ARG LDFLAGS="-w -s"

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.sum ./
COPY main.go ./

RUN go mod download

# Copy the sources
COPY assets/ assets/
COPY cmd/ cmd/
COPY deployments/ deployments/
COPY embedded-files/ embedded-files/
COPY helpers/ helpers/
COPY kubernetes kubernetes/
COPY paas paas/
COPY statik statik/

RUN go get github.com/rakyll/statik

# Build
RUN statik -m -f -src=./embedded-files
RUN CGO_ENABLED=0 GO111MODULE=on go build -a -ldflags "$LDFLAGS" -o dist/fuseml-installer

# Move the binary to smaller image
FROM alpine
WORKDIR /
COPY --from=builder /workspace/dist/fuseml-installer .

ENV KUBE_LATEST_VERSION="v1.20.5"
ENV HELM_VERSION="3.5.4"

# git, kubectl, helm and bash (for codeskyblue/kexec) are required by fuseml-installer
# curl is needed not just for helm and kubectl, but by the istio installation script downloded by fuseml-installer
RUN apk add --update curl git bash openssl \
  && export ARCH="$(uname -m)" \
  && export OS=$(uname|tr '[:upper:]' '[:lower:]') \
  && if [[ ${ARCH} == "x86_64" ]]; then export ARCH="amd64"; fi \
  && if [[ ${ARCH} == "aarch64" ]]; then export ARCH="arm64"; fi \
  && curl -L https://storage.googleapis.com/kubernetes-release/release/${KUBE_LATEST_VERSION}/bin/${OS}/${ARCH}/kubectl -o /usr/local/bin/kubectl \
  && chmod +x /usr/local/bin/kubectl \
  && curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 \
  && chmod 700 get_helm.sh \
  && ./get_helm.sh \
  && rm get_helm.sh /var/cache/apk/*

ENTRYPOINT ["/fuseml-installer"]
