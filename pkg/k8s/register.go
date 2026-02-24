package k8s

import (
	"knative.dev/func/pkg/deployer"
	fn "knative.dev/func/pkg/functions"
)

// Register adds the Kubernetes (raw) deployer to the given registry.
func Register(r *fn.Registry) {
	r.RegisterDeployer(KubernetesDeployerName, k8sFactory)
}

func k8sFactory(cfg fn.DeployerConfig) []fn.Option {
	var opts []DeployerOpt
	opts = append(opts, WithDeployerVerbose(cfg.Verbose))
	if cfg.Decorator != nil {
		opts = append(opts, WithDeployerDecorator(decoratorAdapter{cfg.Decorator}))
	}
	return []fn.Option{fn.WithDeployer(NewDeployer(opts...))}
}

// decoratorAdapter bridges fn.DeployDecorator to deployer.DeployDecorator.
type decoratorAdapter struct{ d fn.DeployDecorator }

func (a decoratorAdapter) UpdateAnnotations(f fn.Function, aa map[string]string) map[string]string {
	return a.d.UpdateAnnotations(f, aa)
}
func (a decoratorAdapter) UpdateLabels(f fn.Function, ll map[string]string) map[string]string {
	return a.d.UpdateLabels(f, ll)
}

var _ deployer.DeployDecorator = decoratorAdapter{}
