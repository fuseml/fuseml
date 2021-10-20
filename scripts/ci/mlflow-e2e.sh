#!/bin/bash

set -e
set -u
set -o pipefail

CODESETS="sklearn tensorflow onnx"
WORKFLOW="mlflow-e2e"
PREDICTION_ENGINE="kfserving"
: ${RELEASE_BRANCH:="main"}

print_bold() {
    echo
    echo -e "\033[1m$1\033[0m"
}

if [ "${1-}" == "seldon" ] ; then
    WORKFLOW="mlflow-seldon-e2e"
    PREDICTION_ENGINE="seldon"
    CODESETS="sklearn tensorflow"
elif [ "${1-}" == "ovms" ] ; then
    WORKFLOW="mlflow-ovms-e2e"
    PREDICTION_ENGINE="ovms"
    CODESETS="keras"
fi

wait_for_run() {
    retries=$1
    codeset=$2
    run_name=$(./fuseml workflow list-runs --name ${WORKFLOW} --codeset-name ${codeset} --format json | jq -r ".[0].name")
    for i in $(seq ${retries}); do
        if [[ "${i}" == ${retries} ]]; then
            print_bold "❌  Timeout waiting for the workflow run to finish."
            kubectl get pipelineruns ${run_name} -n fuseml-workloads -o json | jq ".status"
            exit 1
        fi

        run_status=$(./fuseml workflow list-runs --name ${WORKFLOW} --codeset-name ${codeset} --format json | jq -r ".[0].status")
        echo "  [${codeset}] status: ${run_status} (${i}/$((retries-1)))"

        case ${run_status} in
            Unknown|Started|Running) sleep 15 ;;
            Failed)
                print_bold "❌  Workflow run failed: "
                kubectl get pipelineruns ${run_name} -n fuseml-workloads -o json | jq ".status"
                failed=$(kubectl get pods -n fuseml-workloads --field-selector=status.phase=Failed -o jsonpath='{.items[0].metadata.name}')
                kubectl describe pod $failed -n fuseml-workloads
                kubectl logs $failed -n fuseml-workloads
                exit 1
                ;;
            *) break ;;
        esac
    done
}

# when running for a release, assume the installer already fetched the latest released CLI version,
# otherwise build the CLI from the corresponding release or main branch
if ! git describe --tags --exact-match &> /dev/null; then
    print_bold "➤ Build FuseML client:"
    if [ -d "fuseml-core" ]; then
        cd fuseml-core
        git fetch origin
        git reset --hard origin/${RELEASE_BRANCH}
        git clean -f -d
    else
        git clone -b ${RELEASE_BRANCH} https://github.com/fuseml/fuseml-core.git
        cd fuseml-core
    fi
    make deps generate build_client
    mv bin/fuseml ../fuseml
    cd ..
    rm -rf fuseml-core
fi


export FUSEML_SERVER_URL=http://$(kubectl get VirtualService -n fuseml-core fuseml-core -o jsonpath="{.spec.hosts[0]}")
print_bold "⛓  FuseML URL: ${FUSEML_SERVER_URL}"
./fuseml version

print_bold "➤ Checkout examples repository:"
if [ -d "fuseml-examples" ]; then
    cd fuseml-examples
    git fetch origin
    git reset --hard origin/${RELEASE_BRANCH}
    git clean -f -d
    cd ..
else
    git clone -b ${RELEASE_BRANCH} https://github.com/fuseml/examples.git fuseml-examples
fi

for cs in ${CODESETS}; do
    print_bold "➤ Register Codeset: ${cs}"
    ./fuseml codeset register --name ${cs} --project mlflow fuseml-examples/codesets/mlflow/${cs}
done

print_bold "➤ Create Workflow: ${WORKFLOW}"
# Delete workflow if already exists
if ./fuseml workflow get -n ${WORKFLOW} &> /dev/null; then
    ./fuseml workflow delete -n ${WORKFLOW}
fi
./fuseml workflow create fuseml-examples/workflows/${WORKFLOW}.yaml

for cs in $CODESETS; do
    print_bold "➤ Assign Workflow to Codeset: ${cs}"
    ./fuseml workflow assign --name ${WORKFLOW} --codeset-name ${cs} --codeset-project mlflow
    ./fuseml workflow list-runs --name ${WORKFLOW} --codeset-name ${cs} --codeset-project mlflow
done

retries=181
print_bold "⏱  Waiting $(((retries-1)*15/60))m for workflow runs to finish..."
for cs in ${CODESETS}; do
    wait_for_run ${retries} ${cs} &
done
wait

print_bold "➤ List Applications:"
./fuseml application list

for cs in ${CODESETS}; do
    PREDICTION_URL=$(./fuseml application get -n mlflow-${cs} --format json | jq -r ".url")
    print_bold "⛓  [${cs}] Prediction URL: ${PREDICTION_URL}"

    print_bold "➤ Perform prediction:"
    data="fuseml-examples/prediction/data-${cs}.json"
    if [ "${PREDICTION_ENGINE}" = "seldon" -a "${cs}" = "sklearn" ]; then
        data="fuseml-examples/prediction/data-${cs}-seldon.json"
    elif [ "${PREDICTION_ENGINE}" = "ovms" ]; then
        data="fuseml-examples/prediction/data-${cs}-ovms.json"
    fi
    curl -sd @${data} ${PREDICTION_URL} -H "Accept: application/json" -H "Content-Type: application/json" | jq

    prediction=$(curl -sd @${data} $PREDICTION_URL -H "Accept: application/json" -H "Content-Type: application/json")

    case ${cs} in
        sklearn|onnx)
            if [ "${PREDICTION_ENGINE}" == "seldon" ]; then
                result=$(jq -r ".data.ndarray[0]" <<< ${prediction})
            else 
                result=$(jq -r ".outputs[0].data[0]" <<< ${prediction})
            fi
            expected_result="6.4863448"
            ;;
        tensorflow)
            result=$(jq -r ".predictions[0].all_classes[0]" <<< ${prediction})
            expected_result="0"
            ;;
        keras)
            result=$(jq -r ".predictions[0][]" <<< "${prediction}" | wc -l | tr -d ' ')
            expected_result="10"
    esac

    if [[ "$result" != "${expected_result}"* ]]; then
        print_bold "❌  Prediction result expected ${expected_result} but got ${result}"
        exit 1
    fi
done
