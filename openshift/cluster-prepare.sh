#!/usr/bin/env bash
#
# Prepare Cluster on Openshift CI
# - Creates testing Namespace
# - Setup Openshift Serverless and Openshift Pipelines
# - Creates Test GitServer service

set -o errexit
set -o nounset
set -o pipefail

BASEDIR=$(dirname "$0")
INSTALL_SERVERLESS="${INSTALL_SERVERLESS:-true}"
INSTALL_PIPELINES="${INSTALL_PIPELINES:-true}"
INSTALL_GITSERVER="${INSTALL_GITSERVER:-true}"
GITSERVER_IMAGE="${GITSERVER_IMAGE:-ghcr.io/jrangelramos/gitserver-unpriv:latest}"

go env
source "$(go run knative.dev/hack/cmd/script e2e-tests.sh)"

# Prepare Namespace
TEST_NAMESPACE="${TEST_NAMESPACE:-knfunc-oncluster-test-$(head -c 128 </dev/urandom | LC_CTYPE=C tr -dc 'a-z0-9' | fold -w 6 | head -n 1)}"
oc new-project "${TEST_NAMESPACE}" || true
oc project "${TEST_NAMESPACE}"

# Installs Openshift Serverless
if [ "$INSTALL_SERVERLESS" == "true" ] ; then
  header "Installing Openshift Serverless"
  oc apply -f "${BASEDIR}/deploy/serverless-subscription.yaml"
  wait_until_pods_running openshift-serverless

  subheader "Installing Serving and Eventing"
  oc apply -f "${BASEDIR}/deploy/knative-serving.yaml"
  oc apply -f "${BASEDIR}/deploy/knative-eventing.yaml"
  oc wait --for=condition=Ready --timeout=10m knativeserving knative-serving -n knative-serving
  oc wait --for=condition=Ready --timeout=10m knativeeventing knative-eventing -n knative-eventing
fi

# Installs Openshift Pipelines
if [ "$INSTALL_PIPELINES" == "true" ] ; then
  header "Installing Openshift Pipelines"
  oc apply -f "${BASEDIR}/deploy/pipelines-subscription.yaml"
  wait_until_pods_running openshift-pipelines
fi

# Installs Test Git Server
if [ "$INSTALL_GITSERVER" == "true" ] ; then
  header "Installing Test GitServer"
  sed "s!_GITSERVER_IMAGE_!${GITSERVER_IMAGE}!g" "${BASEDIR}/deploy/gitserver-service.yaml" > "${BASEDIR}/deploy/gitserver.yaml"
  oc apply -f "${BASEDIR}/deploy/gitserver.yaml"
  oc wait pod/gitserver --for=condition=Ready --timeout=15s

  subheader "Exposing Test GitServer route"
  oc expose service gitserver --name=gitserver --port=8080
fi
