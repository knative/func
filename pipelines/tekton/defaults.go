package tekton

const (
	// DefaultPersistentVolumeClaimSize represents default size of PVC created for a Pipeline,
	// specified in bytes (eg. 256Mi = 256MiB = 256 * 1024 * 1024)
	DefaultPersistentVolumeClaimSize int64 = 256 * 1024 * 1024
)
