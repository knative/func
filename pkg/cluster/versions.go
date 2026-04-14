package cluster

// Component versions — Kubernetes and Knative ecosystem components installed
// into the cluster. Source of truth previously split across
// hack/component-versions.json and hardcoded values in hack/cluster.sh.
const (
	kindNodeVersion      = "v1.34.0@sha256:7416a61b42b1662ca6ca89f02028ac133a309a2a30ba309614e8ec94d976dc5a"
	servingVersion       = "v1.21.2"
	eventingVersion      = "v1.21.2"
	contourVersion       = "v1.21.1"
	tektonVersion        = "v1.1.0"
	pacVersion           = "v0.35.2"
	kedaVersion          = "v2.17.0"
	kedaHTTPAddOnVersion = "v0.12.0"
	metalLBVersion       = "v0.13.7"
)

// Tool versions — only tools we download and manage.
const (
	kubectlVersion = "1.33.1"
	kindVersion    = "0.31.0"
)

// kubectlChecksums pins the expected SHA-256 of the kubectl binary for each
// supported os/arch at kubectlVersion. Update in lockstep with kubectlVersion.
// Sourced from https://dl.k8s.io/v<version>/bin/<os>/<arch>/kubectl.sha256.
var kubectlChecksums = map[string]string{
	"linux/amd64":  "5de4e9f2266738fd112b721265a0c1cd7f4e5208b670f811861f699474a100a3",
	"linux/arm64":  "d595d1a26b7444e0beb122e25750ee4524e74414bbde070b672b423139295ce6",
	"darwin/amd64": "8d36a5c66142547ad16e332942fd16a0ca2b3346d9ebaab6c348de2c70d9d875",
	"darwin/arm64": "8ae6823839993bb2e394c3cf1919748e530642c625dc9100159595301f53bdeb",
}

// kindChecksums pins the expected SHA-256 of the kind binary for each
// supported os/arch at kindVersion. Update in lockstep with kindVersion.
// Sourced from the kind-<os>-<arch>.sha256sum files on the GitHub release.
var kindChecksums = map[string]string{
	"linux/amd64":  "eb244cbafcc157dff60cf68693c14c9a75c4e6e6fedaf9cd71c58117cb93e3fa",
	"linux/arm64":  "8e1014e87c34901cc422a1445866835d1e666f2a61301c27e722bdeab5a1f7e4",
	"darwin/amd64": "a8b3cf77b2ad77aec5bf710d1a2589d9117576132af812885cad41e9dede4d4e",
	"darwin/arm64": "88bf554fe9da6311c9f8c2d082613c002911a476f6b5090e9420b35d84e70c5c",
}
