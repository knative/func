module knative.dev/kn-plugin-func

go 1.15

require (
	github.com/AlecAivazis/survey/v2 v2.2.14
	github.com/Masterminds/semver v1.5.0
	github.com/Netflix/go-expect v0.0.0-20210722184520-ef0bf57d82b3
	github.com/alecthomas/jsonschema v0.0.0-20210526225647-edb03dcab7bc
	github.com/buildpacks/pack v0.21.1
	github.com/cloudevents/sdk-go/v2 v2.5.0
	github.com/containers/image/v5 v5.10.6
	github.com/docker/docker v20.10.8+incompatible
	github.com/docker/docker-credential-helpers v0.6.4
	github.com/docker/go-connections v0.4.0
	github.com/go-git/go-billy/v5 v5.3.1
	github.com/go-git/go-git/v5 v5.4.2
	github.com/google/go-cmp v0.5.6
	github.com/google/uuid v1.2.0
	github.com/hinshun/vt10x v0.0.0-20180809195222-d55458df857c
	github.com/markbates/pkger v0.17.1
	github.com/mitchellh/go-homedir v1.1.0
	github.com/opencontainers/image-spec v1.0.2-0.20190823105129-775207bd45b6
	github.com/ory/viper v1.7.5
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v1.2.1
	golang.org/x/mod v0.5.1 // indirect
	golang.org/x/net v0.0.0-20210929193557-e81a3d93ecf6 // indirect
	golang.org/x/sys v0.0.0-20211002104244-808efd93c36d // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/tools v0.1.7 // indirect
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.20.7
	k8s.io/apimachinery v0.20.7
	k8s.io/client-go v0.20.7
	knative.dev/client v0.24.0
	knative.dev/eventing v0.24.0
	knative.dev/hack v0.0.0-20210622141627-e28525d8d260
	knative.dev/pkg v0.0.0-20210622173328-dd0db4b05c80
	knative.dev/serving v0.24.0
)

// knative.dev/serving@v0.21.0 and knative.dev/pkg@v0.0.0-20210331065221-952fdd90dbb0 require different versions of go-openapi/spec
replace (
	// temporary set higher version of buildpacks/imgutil to get better performance for podman
	// rever this once there will be buildpacks/pack with newer version
	github.com/buildpacks/imgutil => github.com/buildpacks/imgutil v0.0.0-20211001201950-cf7ae41c3771
	github.com/go-openapi/spec => github.com/go-openapi/spec v0.19.6
)
