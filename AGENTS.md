# AGENTS.md

## OVERRIDE

Use `AGENTS.override.md` file WITH HIGHEST PRECEDENCE if it exists. Do NOT mix
instructions from both files together. The `AGENTS.override.md` is a complete
override for `AGENTS.md`. Otherwise, continue.

## ALWAYS keep in mind

Don't sugar coat anything. Act like a professional software developer and
engineer. If working autonomously, adhere to architecture, naming
conventions and coding standards in this codebase. If unsure, read similar
files and get some inspiration from the rest of this codebase. If introducing
new features, make sure to cover them via unit tests and don't forget to take
edge cases into account.

## Commands

### Build & Development
- `make all` - Build binary and regenerate docs
- `make build` - Build for current OS

### Testing strategy reference
Before committing, test locally following the table below:

| If changed | Target | Description |
|------------|--------|-------------|
| `*.go` files | `make test` | Core unit tests |
| Any files | `make check` | Linting, formatting, spelling |
| `templates/` files | `make test-templates` | All language template tests |
| Significant architectural changes | `make test-full` | e2e tests (cluster required - read `CONTRIBUTING.md`) |

### Generated Files
- `./hack/update-codegen.sh` - update embedded filesystem & regenerate docs
- `make check-embedded-fs` - check embedded FS is up to date with templates


## Boundaries

### Always Do
- Run `make test` before considering any change complete
- Run `make check` before commits
- Run `make check-embedded-fs` after modifying `templates/`
- Ask before deleting ANY file or significant code block

### Ask First
- Security-related code changes (authentication, credentials, secrets handling)
- API changes
- Adding new dependencies
- Modifying CI/GitHub Actions workflows
- Architectural decisions

### Never Do
- Edit generated files directly:
  - `generate/zz_filesystem_generated.go`
  - `schema/func_yaml-schema.json`
- Commit secrets, API keys, or credentials
- Delete files without explicit user approval
- Force push to main/master branch
- Skip tests or linting

## Common Pitfalls

### Codegen Sync
After modifying `templates/` or making documentation changes, you MUST run:
```bash
./hack/update-codegen.sh
```


## Contributing

If creating a contribution to this project on GitHub suggest to user reading
CONTRIBUTING.md.
