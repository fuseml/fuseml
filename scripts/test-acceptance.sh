#!/bin/bash

# Don't set FUSEML_ACCEPTANCE_KUBECONFIG when using multiple ginkgo
# nodes because they will all use the same cluster. This will lead to flaky
# tests.
if [ -z ${FUSEML_ACCEPTANCE_KUBECONFIG+x} ]; then
  ginkgo -stream acceptance/. ${@}
else
  ginkgo acceptance/. ${@}

fi
