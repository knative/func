---
allowed-tools: Bash(git add:*), Bash(git status:*), Bash(git commit:*), Bash(git log:*)
description: Create a git commit
---

## Context

- Current git status: !`git status`
- Current git diff (staged and unstaged changes): !`git diff HEAD`
- Current branch: !`git branch --show-current`
- Recent commits: !`git log --oneline -10`

## Your Task

- Summarize the changes of staged files into a concise commit message.
- Following conventional commits spec.
- Use the style of writing as I (Stanislav Jakuschevskij) do.
- Maximum subject width is 50 characters.
- Maximum body width is 90 characters which means break the line after 90 characters.
- Add issues reference at the bottom: "Issue $ARGUMENTS"
- Show me the final message in the end, don't commit it.
