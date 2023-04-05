package tekton

import (
	"context"
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"knative.dev/func/pkg/docker"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/git"
	"knative.dev/func/pkg/git/github"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/pipelines"
	"knative.dev/func/pkg/pipelines/tekton/pac"
	"knative.dev/func/pkg/random"
)

// ConfigurePAC cofigures Pipelines as Code resources based on the input:
// - locally (create .tekton directory)
// - on cluster (create Repository, Secret, PVC...)
// - on remote git repo (webhook)
// Parameter `metadata` is type `any` to not bring `pkg/pipelines` package dependency to `pkg/functions`,
// this specific implementation expects the parameter to be a type `pipelines.PacMetada`.
func (pp *PipelinesProvider) ConfigurePAC(ctx context.Context, f fn.Function, metadata any) error {
	data, ok := metadata.(pipelines.PacMetadata)
	if !ok {
		return fmt.Errorf("incorrect type of pipelines metadata: %T", metadata)
	}

	if err := validatePipeline(f); err != nil {
		return err
	}

	if data.ConfigureLocalResources {
		if err := pp.createLocalResources(ctx, f); err != nil {
			return err
		}
	}

	if data.ConfigureClusterResources || data.ConfigureRemoteResources {
		if data.WebhookSecret == "" {
			data.WebhookSecret = random.AlphaString(10)

			// try to reuse existing Webhook Secret stored in the cluster
			secret, err := k8s.GetSecret(ctx, getPipelineSecretName(f), pp.namespace)
			if err != nil {
				if !k8serrors.IsNotFound(err) {
					return err
				}
			} else {
				val, ok := secret.StringData["webhook.secret"]
				if ok {
					data.WebhookSecret = val
				}
			}
		}
	}

	if data.ConfigureClusterResources {
		if err := pp.createClusterResources(ctx, f, data); err != nil {
			return err
		}
	}

	if data.ConfigureRemoteResources {
		if err := pp.createRemoteResources(ctx, f, data); err != nil {
			return err
		}
	}

	return nil
}

// RemovePAC tries to remove all local and remote resources that were created for PAC.
// Resources on the remote GitHub repo are not removed, we would need to store webhook id somewhere locally.
func (pp *PipelinesProvider) RemovePAC(ctx context.Context, f fn.Function, metadata any) error {
	data, ok := metadata.(pipelines.PacMetadata)
	if !ok {
		return fmt.Errorf("incorrect type of pipelines metadata: %T", metadata)
	}

	compoundErrMsg := ""

	if data.ConfigureLocalResources {
		errMsg := deleteAllPipelineTemplates(f)
		compoundErrMsg += errMsg
	}

	if data.ConfigureClusterResources {
		errMsg := pp.removeClusterResources(ctx, f)
		compoundErrMsg += errMsg

	}

	if compoundErrMsg != "" {
		return fmt.Errorf("%s", compoundErrMsg)
	}

	return nil
}

// createLocalResources creates necessary local resources in .tekton directory:
// Pipeline and PipelineRun templates
func (pp *PipelinesProvider) createLocalResources(ctx context.Context, f fn.Function) error {
	err := createPipelineTemplate(f)
	if err != nil {
		return err
	}

	err = createPipelineRunTemplate(f)
	if err != nil {
		return err
	}

	return nil
}

// createClusterResources create resources on cluster, it tries to detect PAC installation,
// creates necessary secret with image registry credentials and git credentials (access tokens, webhook secrets),
// also creates PVC for the function source code
func (pp *PipelinesProvider) createClusterResources(ctx context.Context, f fn.Function, metadata pipelines.PacMetadata) error {
	// figure out pac installation namespace
	installed, _, err := pac.DetectPACInstallation(ctx, "")
	if !installed {
		return fmt.Errorf("pipelines as code not installed: %w", err)
	}
	if installed && err != nil {
		return err
	}

	// let's specify labels that will be applied to every resource that is created for a Pipeline
	labels, err := f.LabelsMap()
	if err != nil {
		return err
	}
	if pp.decorator != nil {
		labels = pp.decorator.UpdateLabels(f, labels)
	}

	registry, err := docker.GetRegistry(f.Image)
	if err != nil {
		return fmt.Errorf("problem in resolving image registry name: %w", err)
	}

	if registry == name.DefaultRegistry {
		registry = authn.DefaultAuthKey
	}

	creds, err := pp.credentialsProvider(ctx, f.Image)
	if err != nil {
		return err
	}

	metadata.RegistryUsername = creds.Username
	metadata.RegistryPassword = creds.Password
	metadata.RegistryServer = registry

	err = createPipelinePersistentVolumeClaim(ctx, f, pp.namespace, labels)
	if err != nil {
		return err
	}
	fmt.Printf(" ✅ Persistent Volume is present on the cluster with name %q\n", getPipelinePvcName(f))

	err = ensurePACSecretExists(ctx, f, pp.namespace, metadata, labels)
	if err != nil {
		return err
	}
	fmt.Printf(" ✅ Credentials are present on the cluster in secret %q\n", getPipelineSecretName(f))

	err = ensurePACRepositoryExists(ctx, f, pp.namespace, metadata, labels)
	if err != nil {
		return err
	}
	fmt.Printf(" ✅ Webhook with payload validation secret %q is present on the cluster in repository %q\n", metadata.WebhookSecret, getPipelineRepositoryName(f))

	return nil
}

// createRemoteResources creates resources on the remote git repository
// set up a webhook with secrets, access tokens and it tries to detec PAC installation
// together with PAC controller route url - needed for webhook payload trigger
func (pp *PipelinesProvider) createRemoteResources(ctx context.Context, f fn.Function, metadata pipelines.PacMetadata) error {
	repoOwner, repoName, err := git.RepoOwnerAndNameFromUrl(f.Build.Git.URL)
	if err != nil {
		return err
	}

	// figure out pac installation namespace
	installed, installationNS, err := pac.DetectPACInstallation(ctx, "")
	if !installed {
		return fmt.Errorf("pipelines as code not installed: %w", err)
	}
	if installed && err != nil {
		return err
	}

	// fetch configmap to get controller url
	controllerURL, err := pac.GetPACInfo(ctx, installationNS)
	if err != nil {
		return err
	}

	// check if info configmap has url then use that otherwise try to detect
	if controllerURL == "" {
		controllerURL, _ = pac.DetectPACOpenShiftRoute(ctx, installationNS)
	}

	// we haven't been able to detect PAC controller public route, let's prompt:
	if controllerURL == "" {
		if err := survey.AskOne(&survey.Input{
			Message: "Please enter your Pipelines As Code controller public route URL: ",
		}, &controllerURL, survey.WithValidator(survey.Required)); err != nil {
			return err
		}
	}

	if err := github.CreateGitHubWebhook(ctx, repoOwner, repoName, controllerURL, metadata.WebhookSecret, metadata.PersonalAccessToken); err != nil {
		// Error: POST https://api.github.com/repos/foobar/test-function/hooks: 422 Validation Failed [{Resource:Hook Field: Code:custom Message:Hook already exists on this repository}]
		if !strings.Contains(err.Error(), "Hook already exists") {
			return err
		}
		fmt.Printf(" ✅ Webhook already exists on repository %v/%v\n", repoOwner, repoName)
	} else {
		fmt.Printf(" ✅ Webhook is created on repository %v/%v\n", repoOwner, repoName)
	}

	return nil
}
