# Functions E2E Test

## Lifecycle tests

Lifecycle tests exercises the most important phases of a function lifecycle starting from
creation, going thru to build, deployment, execution and then deletion (CRUD operations).
It runs func commands such as `create`, `deploy`, `list` and `delete` for a language
runtime using both default `http` and `cloudevents` templates.

## Extended tests

Extended tests performs additional tests on `func` such as templates, config envs, volumes, labels and
other scenarios.

## On Cluster Builds tests

On cluster builds e2e tests exercises functions built directly on cluster.
The tests are organized per scenarios under `./_oncluster` folder.

### Pre-requisites

Prior to run On Cluster builds e2e tests ensure you are connected to
a Kubernetes Cluster with the following deployed:

- Knative Serving
- Tekton
- Tekton Tasks listed [here](../docs/reference/on_cluster_build.md)
- Embedded Git Server (`func-git`) used by tests

For your convenience you can run the following script to setup Tekton and required Tasks:
```
$ ../hack/tekton.sh
```

To install the Git Server required by tests, run:
```
$ ./gitserver.sh
```

#### Running all the Tests on KinD

The below instructions will run all the tests on KinD using an **ephemeral** container registry.
```
# Pre-Reqs
./hack/allocate.sh
./hack/tekton.sh
./test/gitserver.sh
make build

# Run tests
./test/e2e_oncluter_tests.sh
```

#### Running "runtime" only scenario

You can run only e2e tests to exercise a given language/runtime, for example *python*

```
env E2E_RUNTIMES=python TEST_TAGS=runtime ./test/e2e_oncluster_test.sh
```

#### Running tests except "runtime" ones

You can run most of on cluster builds e2e scenarios, except the language/runtime specific
ones, by running:
```
env E2E_RUNTIMES="" ./test/e2e_oncluster_test.sh
```
