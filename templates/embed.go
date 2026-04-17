package templates

import "embed"

//go:embed all:go all:node all:python all:quarkus all:rust
//go:embed all:springboot all:typescript all:certs
//go:embed manifest.yaml README.md .permissions
var Content embed.FS
