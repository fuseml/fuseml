#!/bin/bash

if [ "$(kubectl get job default-domain -n knative-serving -o jsonpath='{.status.conditions[0].type}')" = "Complete" ] ; then
  echo "knative controller already installed"
  exit
fi

export KNATIVE_VERSION=v0.21.0

# Install Knative
kubectl apply --filename https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-crds.yaml
kubectl apply --filename https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-core.yaml
kubectl apply --filename https://github.com/knative/net-istio/releases/download/${KNATIVE_VERSION}/release.yaml
curl -L https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-default-domain.yaml | sed 's/xip/nip/g' | kubectl apply -f -

kubectl wait --for=condition=complete --timeout=600s job/default-domain -n knative-serving
