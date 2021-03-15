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
kubectl apply --filename https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-default-domain.yaml

# Set the knative default revision timeout from 5 minutes to 1 minute as this
# value is used as temrinationGracePeriod on the pod and it is making deleting
# knative services take at least 5 minutes.
# There is a issue upstream about this and it should be fixed
# see: https://github.com/knative/serving/issues/3355
cat << EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
 name: config-defaults
 namespace: knative-serving
data:
  revision-timeout-seconds: "60"
EOF

kubectl wait --for=condition=complete --timeout=600s job/default-domain -n knative-serving
