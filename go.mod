module github.com/boson-project/faas

go 1.14

require (
	github.com/buildpacks/pack v0.14.0
	github.com/markbates/pkger v0.17.1
	github.com/mitchellh/go-homedir v1.1.0
	github.com/ory/viper v1.7.4
	github.com/spf13/cobra v1.0.1-0.20201006035406-b97b5ead31f7
	golang.org/x/sys v0.0.0-20200930185726-fdedc70b468f // indirect
	gopkg.in/yaml.v2 v2.3.0
	k8s.io/api v0.18.8
	k8s.io/apimachinery v0.19.1
	k8s.io/client-go v11.0.1-0.20190805182717-6502b5e7b1b5+incompatible
	knative.dev/client v0.17.0
	knative.dev/eventing v0.17.5
	knative.dev/serving v0.17.3
)

replace (
	// Replace with the version used in docker to overcome an issue with renamed
	// packages (and old docker versions)
	github.com/Sirupsen/logrus => github.com/sirupsen/logrus v1.7.0

	// This is required to pin the docker module to that version that build packs are requiring
	// Otherwise it's overwritten by knative-dev/test-infra to a version v.1.13 that is higher
	github.com/docker/docker => github.com/docker/docker v1.4.2-0.20200221181110-62bd5a33f707

	// Needed for macos
	golang.org/x/sys => golang.org/x/sys v0.0.0-20200302150141-5c8b2ff67527

	// Nail down k8 deps to align with transisitive deps
	k8s.io/apimachinery => k8s.io/apimachinery v0.17.6
	k8s.io/client-go => k8s.io/client-go v0.17.6
)
