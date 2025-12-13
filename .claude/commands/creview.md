---
allowed-tools: Bash(git status:*),  Bash(git log:*)
description: Perform a code review
---

## Context

- Current git status: !`git status`
- Current git diff (staged and unstaged changes): !`git diff HEAD`
- Current branch: !`git branch --show-current`

## Your Task

- Propose refactorings as documented by Martin Fowler in his books if code becomes easier to change with them.
- Propose missing test cases for the implemented logic.
- Point out for Golang security bugs.
- Point out for general code bugs.
- Apply the SOLID principles as documented by Robert C. Martin if the code becomes easier to change with them.
- Propose refactorings towards design patterns as explained in the book "Refactoring to Patterns" by Joshua Kerievsky if they apply in the current context and will improve the maintainability of the code.
