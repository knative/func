module knative.dev/kn-plugin-func

go 1.16

require (
	github.com/AlecAivazis/survey/v2 v2.3.2
	github.com/Masterminds/semver v1.5.0
	github.com/Netflix/go-expect v0.0.0-20210722184520-ef0bf57d82b3
	github.com/alecthomas/jsonschema v0.0.0-20210526225647-edb03dcab7bc
	github.com/buildpacks/pack v0.24.0
	github.com/cloudevents/sdk-go/v2 v2.8.0
	github.com/containerd/containerd v1.6.0
	github.com/containers/image/v5 v5.19.1
	github.com/coreos/go-semver v0.3.0
	github.com/docker/cli v20.10.12+incompatible
	github.com/docker/docker v20.10.12+incompatible
	github.com/docker/docker-credential-helpers v0.6.4
	github.com/docker/go-connections v0.4.0
	github.com/go-git/go-billy/v5 v5.3.1
	github.com/go-git/go-git/v5 v5.4.2
	github.com/google/go-cmp v0.5.7
	github.com/google/go-containerregistry v0.8.1-0.20220414143355-892d7a808387
	github.com/google/uuid v1.3.0
	github.com/hinshun/vt10x v0.0.0-20180809195222-d55458df857c
	github.com/mitchellh/go-homedir v1.1.0
	github.com/opencontainers/image-spec v1.0.3-0.20220114050600-8b9d41f48198
	github.com/openshift/source-to-image v1.3.1
	github.com/ory/viper v1.7.5
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v1.3.0
	github.com/tektoncd/cli v0.23.1
	github.com/tektoncd/pipeline v0.34.1
	github.com/whilp/git-urls v1.0.0
	golang.org/x/crypto v0.0.0-20220214200702-86341886e292
	golang.org/x/term v0.0.0-20210927222741-03fcf44c2211
	gopkg.in/natefinch/npipe.v2 v2.0.0-20160621034901-c1b8fa8bdcce
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.23.5
	k8s.io/apimachinery v0.23.5
	k8s.io/client-go v1.5.2
	k8s.io/utils v0.0.0-20220210201930-3a6ce19ff2f9
	knative.dev/client v0.30.1
	knative.dev/eventing v0.30.4-0.20220601140224-ec844245369e
	knative.dev/hack v0.0.0-20220629135030-82afbd367967
	knative.dev/pkg v0.0.0-20220314165618-a637a96a1bd9
	knative.dev/serving v0.30.3-0.20220630163432-b5f775538f3d
)

replace (
	// Tekton Triggers imports old google/cel-go, should be fixed with tektoncd/cli >=v0.24.x transitively
	github.com/google/cel-go => github.com/google/cel-go v0.11.2
	// update docker to be compatible with version used by pack and removes invalid pseudo-version
	github.com/openshift/source-to-image => github.com/boson-project/source-to-image v1.3.2
	// Pin k8s.io dependencies to align with Knative and Tekton needs
	k8s.io/api => k8s.io/api v0.22.5
	k8s.io/apimachinery => k8s.io/apimachinery v0.22.5
	k8s.io/client-go => k8s.io/client-go v0.22.5
)
