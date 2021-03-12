#!/bin/bash

if kubectl get svc istiod -n istio-system >/dev/null 2>&1 ; then
  echo "istio already installed"
  exit
fi

export ISTIO_VERSION=1.8.2 # https://knative.dev/docs/install/installing-istio/

curl -L https://git.io/getLatestIstio | sh -
cd istio-${ISTIO_VERSION}

# Create istio-system namespace
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: istio-system
  labels:
    istio-injection: disabled
EOF

cat << EOF > ./istio-minimal-operator.yaml
apiVersion: install.istio.io/v1alpha1
kind: IstioOperator
spec:
  values:
    global:
      proxy:
        autoInject: disabled
      useMCP: false

  addonComponents:
    pilot:
      enabled: true

  components:
    ingressGateways:
      - name: istio-ingressgateway
        enabled: true
EOF

bin/istioctl manifest install -yf istio-minimal-operator.yaml
cd ..
rm -rf istio-${ISTIO_VERSION}
