#!/bin/bash

set -e
set -o pipefail

# CLUSTER_NAME is fuse-( branch, or tag, fallback to ref )
CLUSTER_SUFIX=$(git symbolic-ref -q --short HEAD 2>/dev/null || git describe --tags --exact-match --short HEAD 2>/dev/null || git describe --all | tr '/' - | tr _ - )
CLUSTER_NAME=fuseml-${CLUSTER_SUFIX//[!A-Za-z0-9-]/-}

if [ "$1" == "create" ]; then
    k3d_args="--k3s-arg --disable=traefik@server:* --agents 1 \
    --k3s-arg --kubelet-arg=eviction-hard=imagefs.available<5%,nodefs.available<5%@server:* \
    --k3s-arg --kubelet-arg=eviction-minimum-reclaim=imagefs.available=5%,nodefs.available=5%@server:* \
    --k3s-arg --kubelet-arg=eviction-hard=imagefs.available<5%,nodefs.available<5%@agent:* \
    --k3s-arg --kubelet-arg=eviction-minimum-reclaim=imagefs.available=5%,nodefs.available=5%@agent:*"
fi


# Create cluster
echo "k3d cluster $1 ${k3d_args} ${CLUSTER_NAME}"
k3d cluster $1 ${k3d_args} ${CLUSTER_NAME}