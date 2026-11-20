package kafkaruntime

import "embed"

//go:embed all:bin/darwin_amd64
var embedded embed.FS

const embedDir = "bin/darwin_amd64"
