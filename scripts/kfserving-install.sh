#!/bin/bash

if kubectl get svc kfserving-controller-manager-service -n kfserving-system >/dev/null 2>&1 ; then
  echo "kfserving already installed"
  exit
fi

kubectl apply -k scripts/kfserving

kubectl rollout status statefulset/kfserving-controller-manager -n kfserving-system