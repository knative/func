# Building Functions on Cluster with Tekton Pipelines

This guide describes how you can build a Function on Cluster with Tekton Pipelines. The on cluster build is enabled by fetching Function source code from a remote Git repository.

> Please note that the following approach requires administrator privileges on the cluster and the build is executed on a privileged container.

## Prerequisite
1. Install Tekton Pipelines on the cluster. Please refer to [Tekton Pipelines documentation](https://github.com/tektoncd/pipeline/blob/main/docs/install.md) or run the following command:
```bash
kubectl apply -f https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml
```

## Enabling a namespace to run Function related Tekton Pipelines
In each namespace that you would like to run Pipelines and deploy a Function you need to create or install the following resources.
1. Install the Git Clone Tekton Task to fetch the Function source code:
```bash
kubectl apply -f https://raw.githubusercontent.com/tektoncd/catalog/master/task/git-clone/0.4/git-clone.yaml
```
2. Install the Buildpacks Tekton Task to be able to build the Function image:
```bash
kubectl apply -f https://raw.githubusercontent.com/tektoncd/catalog/master/task/buildpacks/0.3/buildpacks.yaml
```
3. Install the `kn func` Deploy Tekton Task to be able to deploy the Function on in the Pipeline:
```bash
kubectl apply -f https://raw.githubusercontent.com/knative-sandbox/kn-plugin-func/main/pipelines/resources/tekton/task/func-deploy/0.1/func-deploy.yaml
```

## Building a Function on Cluster
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
```
4. Update the Function configuration in `func.yaml` to enable on cluster builds for the Git repository:
```yaml
build: git                                          # required, specify `git` build type
git:
  url: https://github.com/my-repo/my-function.git   # required, git repository with the function source code
  revision: main                                    # optional, git revision to be used (branch, tag, commit)
  # contextDir: myfunction                          # optional, needed only if the function is not located
                                                    # in the repository root folder
```
5. Implement the business logic of your Function, then commit and push changes
```bash
git add .
git commit -a -m "implementing my-function"
git push origin main
```
6. Deploy your Function
```bash
kn func deploy
```
If everything goes fine, you will prompted to provide credentials for the remote container registry that hosts the Function image. You should see output similar to the following:
```bash
$ kn func deploy
🕕 Creating Pipeline resources
Please provide credentials for image registry used by Pipeline.
? Server: https://index.docker.io/v1/
? Username: my-repo
? Password: ********
   Function deployed at URL: http://test-function.default.svc.cluster.local
```

7. To update your Function, commit and push new changes, then run `kn func deploy` again.

## Uninstall and clean-up
1. In each namespace where Pipelines and Functions were deployed, uninstall following resources:
```bash
export NAMESPACE=<INSERT_YOUR_NAMESPACE>
kubectl delete serviceaccount knative-deployer-account -n $NAMESPACE
kubectl delete clusterrolebinding $NAMESPACE:knative-deployer-binding
kubectl delete task.tekton.dev git-clone
kubectl delete task.tekton.dev buildpacks
```
2. Delete Knative Deployer Cluster Role
```bash
kubectl delete clusterrole kn-deployer
```
3. Uninstall Tekton Pipelines
```bash
kubectl delete -f https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml
```
