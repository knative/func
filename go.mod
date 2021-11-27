module knative.dev/kn-plugin-func

go 1.16

require (
	github.com/AlecAivazis/survey/v2 v2.2.14
	github.com/Masterminds/semver v1.5.0
	github.com/Netflix/go-expect v0.0.0-20210722184520-ef0bf57d82b3
	github.com/alecthomas/jsonschema v0.0.0-20210526225647-edb03dcab7bc
	github.com/buildpacks/pack v0.22.0
	github.com/cloudevents/sdk-go/v2 v2.5.0
	github.com/containers/image/v5 v5.10.6
	github.com/coreos/go-semver v0.3.0
	github.com/docker/cli v20.10.10+incompatible
	github.com/docker/docker v20.10.10+incompatible
	github.com/docker/docker-credential-helpers v0.6.4
	github.com/docker/go-connections v0.4.0
	github.com/go-git/go-billy/v5 v5.3.1
	github.com/go-git/go-git/v5 v5.4.2
	github.com/google/go-cmp v0.5.6
	github.com/google/uuid v1.3.0
	github.com/hinshun/vt10x v0.0.0-20180809195222-d55458df857c
	github.com/markbates/pkger v0.17.1
	github.com/mitchellh/go-homedir v1.1.0
	github.com/opencontainers/image-spec v1.0.2-0.20190823105129-775207bd45b6
	github.com/ory/viper v1.7.5
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v1.2.1
	golang.org/x/crypto v0.0.0-20210817164053-32db794688a5
	golang.org/x/net v0.0.0-20210929193557-e81a3d93ecf6 // indirect
	golang.org/x/sys v0.0.0-20211002104244-808efd93c36d // indirect
	golang.org/x/term v0.0.0-20210615171337-6886f2dfbf5b
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/tools v0.1.7 // indirect
	k8s.io/api v0.21.4
	k8s.io/apimachinery v0.21.4
	k8s.io/client-go v0.21.4
	knative.dev/client v0.26.0
	knative.dev/eventing v0.26.1
	knative.dev/hack v0.0.0-20210806075220-815cd312d65c
	knative.dev/pkg v0.0.0-20210919202233-5ae482141474
	knative.dev/serving v0.26.0
	sigs.k8s.io/yaml v1.2.0
)

// temporary set higher version of buildpacks/imgutil to get better performance for podman
// rever this once there will be buildpacks/pack with newer version
replace github.com/buildpacks/imgutil => github.com/buildpacks/imgutil v0.0.0-20211001201950-cf7ae41c3771
