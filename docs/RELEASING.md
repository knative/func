# Releasing

The project releases are primarily handled by the Knative [`hack`](https://github.com/knative/hack) infrastructure. When a new branch with a specific naming convention is pushed to GitHub, the Knative Prow system kicks into action, creating a new release. Unfortunately, this does not handle some niceties such as CHANGELOG.md updates. Follow this guide to create a new release, or push a patch for an existing release.

## Prerequisites

You will need Node.js and NPM installed on your system to follow these steps. Managing the CHANGELOG.md and version.txt files is done with the npm package [`standard-version`](https://npmjs.com/package/standard-version).

## Creating a new release branch

Follow these steps to create a new release branch. Release branches use the `release-` prefix, and include the major and minor version numbers, for example, `release-0.23`.

### Steps

When you are ready to create a new release, check out the repository's main branch, and perform the following steps.

- Ensure you are on the most recent commit
```
git checkout main
git fetch
git reset --hard origin/main
```

- Update CHANGELOG.md and version.txt. Running this will command will update both of these files, and commit the changes with a "release" commit. To see what will happen before actually doing it, use the `--dry-run` flag. 
```
standard-version
```

- Create the release branch and push it to GitHub
```
git push origin main:release-X.Y # X is the major version number, Y is the minor version number
```

At this point, prow will see the new branch and begin the release process. Once the release has been created in GitHub, update the release text to include the markdown from the CHANGELOG.txt changes.

## Creating a patch release on an existing release branch

If a release branch already exists, but you need to release a new version with backported bug fixes, follow these steps.

### Steps

When you are ready to create a patch release, check out the release branch to be updated, and perform the following steps.

- Ensure you are on the latest commit from the release branch
```
git checkout release-X.Y # X=major version, Y=minor
git fetch
git reset --hard origin/release-X.Y
```

- Backport the necessary changes/bug fixes onto this branch.
```
git cherry-pick [SHA]
```

- Update CHANGELOG.md and version.txt on this branch. Running this will command will update both of these files, and commit the changes with a "release" commit. To see what will happen before actually doing it, use the `--dry-run` flag. 
```
standard-version
```

- Create the release branch and push it to GitHub
```
git push origin main:release-X.Y # X is the major version number, Y is the minor version number
```

At this point, prow will see the new branch and begin the release process. Once the release has been created in GitHub, update the release text to include the markdown from the CHANGELOG.txt changes.
