package tekton

const (
	// DefaultPersistentVolumeClaimSize represents default size of PVC created for a Pipeline,
	// specified in bytes (eg. 5Mi = 5MiB = 5 * 1024 * 1024)
	DefaultPersistentVolumeClaimSize int64 = 5 * 1024 * 1024
)
