module github.com/buildpacks/imgutil

require (
	github.com/docker/docker v1.4.2-0.20190924003213-a8608b5b67c7
	github.com/docker/go-connections v0.4.0
	github.com/google/go-cmp v0.5.4
	github.com/google/go-containerregistry v0.4.0
	github.com/pkg/errors v0.9.1
	github.com/sclevine/spec v1.4.0
	golang.org/x/crypto v0.0.0-20201016220609-9e8e0b390897
)

replace (
    golang.org/x/sys => golang.org/x/sys v0.0.0-20200523222454-059865788121
)

go 1.14
