#!/bin/bash

set -e
set -o pipefail

# CLUSTER_NAME is fuse-( branch, or tag, fallback to ref )
CLUSTER_SUFIX=$(echo $(git symbolic-ref -q --short HEAD 2>/dev/null || git describe --tags --exact-match --short HEAD 2>/dev/null || git describe --all | tr '/' -) | tr _ - )
CLUSTER_NAME=fuseml-${CLUSTER_SUFIX}

if [ "$1" == "create" ]; then
    k3d_args="--k3s-server-arg --no-deploy=traefik --agents 1"
fi


# Create cluster
k3d cluster $1 ${k3d_args} ${CLUSTER_NAME}