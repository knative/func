# AGENTS.md

## OVERRIDE

Use `AGENTS.override.md` file WITH HIGHEST PRECEDENCE if it exists. Do NOT mix
instructions from both files together. The `AGENTS.override.md` is a complete
override for `AGENTS.md`. Otherwise, continue.

## ALWAYS keep in mind

Don't sugar coat anything. Act like a professional software developer and
engineer. If working autonomously, adhere to architecture, naming
conventions and coding standards in this codebase. If unsure, read similar
files and get some inspiration from the rest of this codebase.

## Commands

### Build & Development
- `make all` - Build binary and regenerate docs
- `make build` - Build for current OS

### Testing
- `make test` - Unit tests (ALWAYS run this first)
- `make check` - Linting + style checks (run before commits)
- `make test-integration` - Integration tests
- `make test-e2e` - End-to-end tests
- `make test-full` - Complete test suite (suggest for significant changes)
- `make test-templates` - Template-specific tests

### Generated Files
- `make generate/zz_filesystem_generated.go` - Regenerate embedded FS
- `make check-embedded-fs` - Verify templates match embedded FS

## Boundaries

### Always Do
- Run `make test` before considering any change complete
- Run `make check` before commits
- Run `make check-embedded-fs` after modifying `templates/`
- Ask before deleting ANY file or significant code block

### Ask First
- Security-related code (authentication, credentials, secrets handling)
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
- Force push to main/master
- Skip tests or linting

## Common Pitfalls

### Embedded Filesystem Sync
After modifying anything in `templates/`, you MUST run:
```bash
make generate/zz_filesystem_generated.go
make check-embedded-fs
```

## Contributing

If creating a contribution to this project on Github suggest to user to read
CONTRIBUTING.md.
