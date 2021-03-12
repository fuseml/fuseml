#!/bin/bash

if kubectl get svc kfserving-controller-manager-service -n kfserving-system >/dev/null 2>&1 ; then
  echo "kfserving already installed"
  exit
fi

export KFSERVING_VERSION=v0.5.1

curl https://raw.githubusercontent.com/kubeflow/kfserving/${KFSERVING_VERSION}/install/${KFSERVING_VERSION}/kfserving.yaml | \
  sed 's/cluster-local/knative-local/' | kubectl apply -f -

kubectl rollout status statefulset/kfserving-controller-manager -n kfserving-system