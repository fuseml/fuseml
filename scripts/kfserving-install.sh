#!/bin/bash

if kubectl get svc kfserving-controller-manager-service -n kfserving-system >/dev/null 2>&1 ; then
  echo "kfserving already installed"
  exit
fi

export KFSERVING_VERSION=v0.5.1

kubectl apply --filename scripts/kfserving/kfserving-${KFSERVING_VERSION}-v1beta1.yaml

kubectl rollout status statefulset/kfserving-controller-manager -n kfserving-system