//go:build !((darwin || linux || windows) && (amd64 || arm64))

package kafkaruntime

import "embed"

// No Kafka runtime is embedded for this platform: Binary returns
// ErrUnavailable unless BinaryEnv is set.
var embedded embed.FS

const embedDir = ""
