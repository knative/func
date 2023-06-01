# Building Functions on Cluster with Tekton Pipelines

This guide describes how you can build a Function on Cluster with Tekton Pipelines. The on cluster build is enabled by fetching Function source code from a remote Git repository. Buildpacks or S2I builder strategy can be used to build the Function image.

> **Note**
> Not all runtimes support on cluster builds. **Go** and **Rust** are not currently supported.

## Prerequisite
1. Install Tekton Pipelines on the cluster. Please refer to [Tekton Pipelines documentation](https://github.com/tektoncd/pipeline/blob/main/docs/install.md) or run the following command:
```bash
kubectl apply -f https://storage.googleapis.com/tekton-releases/pipeline/previous/v0.42.0/release.yaml
```

## Enabling a namespace to run Function related Tekton Pipelines
In each namespace that you would like to run Pipelines and deploy a Function you need to create or install the following resources.
1. Install the Git Clone Tekton Task to fetch the Function source code:
```bash
kubectl apply -f https://raw.githubusercontent.com/tektoncd/catalog/master/task/git-clone/0.4/git-clone.yaml
```
2. Install a Tekton Task responsible for building the Function, based on the builder preference (Buildpacks or S2I)
   1. For Buildpacks builder install the Functions Buildpacks Tekton Task:
      ```bash
      kubectl apply -f https://raw.githubusercontent.com/knative-sandbox/kn-plugin-func/main/pkg/pipelines/resources/tekton/task/func-buildpacks/0.1/func-buildpacks.yaml
      ```
   2. For S2I builder install the S2I task:
      ```bash
      kubectl apply -f https://raw.githubusercontent.com/knative-sandbox/kn-plugin-func/main/pkg/pipelines/resources/tekton/task/func-s2i/0.1/func-s2i.yaml
      ```
3. Install the `kn func` Deploy Tekton Task to be able to deploy the Function on in the Pipeline:
```bash
kubectl apply -f https://raw.githubusercontent.com/knative-sandbox/kn-plugin-func/main/pkg/pipelines/resources/tekton/task/func-deploy/0.1/func-deploy.yaml
```
4. Add permission to deploy on Knative to `default` Service Account: (This is not needed on OpenShift)
```bash
export NAMESPACE=<INSERT_YOUR_NAMESPACE>
kubectl create clusterrolebinding $NAMESPACE:knative-serving-namespaced-admin \
--clusterrole=knative-serving-namespaced-admin  --serviceaccount=$NAMESPACE:default
```

## Create a Function Project for On Cluster Builds
To get started with on cluster builds, you will need to have a function project with a `func.yaml` file that specifies the build type and the Git repository where the function source code is located.

1. Create a Function and implement the business logic
```bash
kn func create my-function
```
2. Get a reference to the remote Git repository that will host your Function source code (eg. `https://github.com/my-repo/my-function.git`)
3. Initialize a Git repository in your Function project and add a reference to the remote repo
```bash
cd my-function
git init
git branch -M main
git remote add origin git@github.com:my-repo/my-function.git
git add .
git commit -am "initial commit"
git push origin main
```
4. Configure the function to enable on cluster builds for the Git repository:
```yaml
func config git set
```
This command will update the `func.yaml` file in your project directory to include the following configuration:
```yaml
build: git                                          # specify the `git` build type
git:
  url: https://github.com/my-repo/my-function.git   # required, git repository with the function source code
  revision: main                                    # optional, git revision to be used (branch, tag, commit)
  # contextDir: myfunction                          # optional, needed only if the function is not located
                                                    # in the repository root folder
```

5. Implement the business logic of your Function, then commit and push changes
```bash
git add .
git commit -am "implementing my-function"
git push origin main
```

## Building and Deploying a Function on Cluster Using the CLI
There are two ways to build a function on your cluster. In this first example, we'll use the `func` CLI to initiate an on-cluster build directly from the project directory.

6. Deploy your Function
```bash
kn func deploy --remote
```
If you are not logged in the container registry referenced in your function configuration,
you will prompted to provide credentials for the remote container registry that hosts the Function image. You should see output similar to the following:
```bash
$ kn func deploy --remote
🕕 Creating Pipeline resources
Please provide credentials for image registry used by Pipeline.
? Server: https://index.docker.io/v1/
? Username: my-repo
? Password: ********
   Function deployed at URL: http://test-function.default.svc.cluster.local
```

7. To update your Function, commit and push new changes, then run `kn func deploy --remote` again.

## Building a Function on the Cluster with Pipelines as Code
The second, more useful way to build a function on your cluster is to use pipelines as code.

Pipelines as code is a feature of Tekton that allows you to define CI/CD for your function using Tekton `Pipelines` and `PipelineRuns` in files stored in your project on GitHub. These files are then used to automatically create a pipeline for a Pull Request or a Push to a branch in the repository.

This approach enables automation and repeatability using a Git workflow.

The `func` CLI can generate the Tekton resources for you, adding them to a `.tekton` directory in your project. You can then commit these files to your project and use them to build your function on the cluster. When you ran `func config git set` earlier, this command also generated a `.tekton/pipeline.yaml` and a `.tekton/pipeline-run.yaml` file in your project directory.

To initiate an on cluster build using the Tekton resources generated by the `func` CLI, you will need to push a commit to the remote repository. This will trigger the pipeline to run on the cluster.

## Uninstall and clean-up
1. In each namespace where Pipelines and Functions were deployed, uninstall following resources:
```bash
export NAMESPACE=<INSERT_YOUR_NAMESPACE>
kubectl delete clusterrolebinding $NAMESPACE:knative-serving-namespaced-admin
kubectl delete task.tekton.dev git-clone
kubectl delete task.tekton.dev func-buildpacks
kubectl delete task.tekton.dev func-s2i
kubectl delete task.tekton.dev func-deploy
```
2. Uninstall Tekton Pipelines
```bash
kubectl delete -f https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml
```
