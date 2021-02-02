module github.com/boson-project/func

go 1.14

require (
	github.com/buildpacks/pack v0.16.0
	github.com/markbates/pkger v0.17.1
	github.com/mitchellh/go-homedir v1.1.0
	github.com/ory/viper v1.7.4
	github.com/spf13/cobra v1.1.1
	gopkg.in/yaml.v2 v2.3.0
	k8s.io/api v0.18.8
	k8s.io/apimachinery v0.19.1
	k8s.io/client-go v11.0.1-0.20190805182717-6502b5e7b1b5+incompatible
	knative.dev/client v0.19.1
	knative.dev/eventing v0.19.0
	knative.dev/pkg v0.0.0-20201103163404-5514ab0c1fdf
	knative.dev/serving v0.19.0
)

replace (
	// Nail down k8 deps to align with transisitive deps
	k8s.io/apimachinery => k8s.io/apimachinery v0.18.8
	k8s.io/client-go => k8s.io/client-go v0.18.8
)
