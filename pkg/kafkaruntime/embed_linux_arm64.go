package kafkaruntime

import "embed"

//go:embed all:bin/linux_arm64
var embedded embed.FS

const embedDir = "bin/linux_arm64"
