package kafkaruntime

import "embed"

//go:embed all:bin/windows_amd64
var embedded embed.FS

const embedDir = "bin/windows_amd64"
