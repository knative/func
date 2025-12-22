---
allowed-tools: Bash(git log:*), Bash(git diff:*)
description: Create a pull request description for Knative func
---

# Knative Func PR Guidelines

Analyze the git commits on the current branch since it diverged from the main branch and create a pull request description following the Knative func PR template below.

## Steps

1. Run `git log main..HEAD` to get all commits on this branch.
2. Run `git diff main...HEAD --stat` to understand the scope of changes.
3. Analyze the commits to determine:
   - What changes were made => extract from commit messages.
   - The /kind label => bug, enhancement, cleanup, documentation, etc..
   - Any issue numbers referenced in commits (for "Fixes #").
   - Whether changes are user-facing (for release notes).

4. Format the PR description using the template in `.github/pull-request-template.md` and do the following:
   - Add a PR title in a comment above `# Changes` like so: `<!-- PR Title: <pull-request-title> -->`.
      - Use correct English capitalization and grammar for the PR title.
      - Replace `<pull-request-title>` with the actual title.
   - Update the "Changes" section with bullet points using appropriate emoji: :gift: :bug: :broom: :wastebasket:.
   - Add a /kind label
   - Add "Fixes #" if issue is referenced or "Relates to #" if issue is not supposed to be closed by this PR.
   - Fill in the "Release Note" if the changes are user-facing otherwise leave it empty.
   - Leave docs section as is.
   - Remove the comments from the final PR description except the comment with the PR title: `<!-- PR Title: ...`.

5. Write the PR description in a file in `./.claude/pr/<pull-request-name>.md` for review and:
   - Create the directory if it does not exist.
   - Replace `<pull-request-name>` in the filename with a short, descriptive pull request name.
