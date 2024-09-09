#!/usr/bin/env bash
#
# This script is executed during upstream/midstream sync to release-next branch
# It removes github actions (workflows) executed by Openshift CI
#

set -o errexit
set -o nounset
set -o pipefail

# Covered by https://github.com/openshift/release/blob/master/ci-operator/config/openshift-knative/kn-plugin-func/openshift-knative-kn-plugin-func-release-next.yaml
rm .github/workflows/test-e2e-oncluster-runtime.yaml
rm .github/workflows/test-e2e-oncluster.yaml
