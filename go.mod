module github.com/boson-project/func

go 1.14

require (
	github.com/buildpacks/pack v0.18.0
	github.com/containers/image/v5 v5.10.5
	github.com/docker/docker v20.10.0-beta1.0.20201110211921-af34b94a78a1+incompatible
	github.com/docker/go-connections v0.4.0
	github.com/markbates/pkger v0.17.1
	github.com/mitchellh/go-homedir v1.1.0
	github.com/ory/viper v1.7.4
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v1.1.3
	golang.org/x/crypto v0.0.0-20201016220609-9e8e0b390897
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.18.12
	k8s.io/apimachinery v0.18.12
	k8s.io/client-go v11.0.1-0.20190805182717-6502b5e7b1b5+incompatible
	knative.dev/client v0.20.0
	knative.dev/eventing v0.20.0
	knative.dev/pkg v0.0.0-20210107022335-51c72e24c179
	knative.dev/serving v0.20.0
)

replace (
	// Nail down k8 deps to align with transisitive deps
	k8s.io/client-go => k8s.io/client-go v0.18.12
	k8s.io/code-generator => k8s.io/code-generator v0.18.12
)
