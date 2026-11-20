package kafkaruntime

import "embed"

//go:embed all:bin/linux_amd64
var embedded embed.FS

const embedDir = "bin/linux_amd64"
