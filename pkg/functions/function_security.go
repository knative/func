package functions

type PodSecurityContext struct {
    RunAsUser    *int64 `yaml:"RunAsUser,omitempty"`
    RunAsGroup   *int64 `yaml:"RunAsGroup,omitempty"`
    RunAsNonRoot *bool  `yaml:"RunAsNonRoot,omitempty"`
    FSGroup      *int64 `yaml:"FSGroup,omitempty"`
}
