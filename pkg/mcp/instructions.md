# Functions MCP Agent Instructions

## Terminology

Always capitalize "**Function**" when referring to a deployable Function (the service). Use lowercase "function" only for programming concepts (functions in code). Suggest users do the same when ambiguity is detected.

**Examples:**
- "Let's create a Function!" (deployable service) ✓
- "What is a Function?" (this project's concept) ✓
- "What is a function?" (programming construct) ✓
- "Let's create a function" (ambiguous - could mean code) ✗

## Workflow Pattern

Functions work like 'git' - you should always BE in the Function directory:

1. Navigate to (cd into) the directory where you want to work
2. Use tools to Read, Edit, etc. to work with files in that directory
3. When invoking MCP tools, ALWAYS pass your current working directory as the 'path' parameter

The func binary is smart - if func.yaml has previous deployment config, the deploy tool can be called with minimal arguments and will reuse registry, builder, etc. In general, arguments need only be used once. Subsequent invocations of the command should "remember" the prior settings as they are populated based on the state of the Function.

## Agent Working Directory Pattern

**CRITICAL: YOU (the agent) should always BE in the Function directory you're working on.**

This is essential because:
- File operations (Read, Edit, Bash, etc.) work relative to YOUR current working directory
- The user will say things like "edit my handler" expecting you to be IN the Function directory
- This matches how developers naturally work with Functions (like git repositories)

**When calling MCP tools, ALWAYS pass the ABSOLUTE path to your current working directory as the 'path' parameter:**
- `create` tool: path = absolute path to directory where Function will be created
- `deploy` tool: path = absolute path to Function directory (where func.yaml exists)
- `build` tool: path = absolute path to Function directory (where func.yaml exists)
- `config_*` tools: path = absolute path to Function directory (where func.yaml exists)

**IMPORTANT:** You must use absolute paths (e.g., `/Users/name/myproject/myfunc`), NOT relative paths (e.g., `.` or `myfunc`). The MCP server process runs in a different directory than your current working directory, so relative paths will not resolve correctly.

**Exceptions:**
- The `list` tool operates on the cluster, not local files, so it does NOT use a path parameter (it uses namespace instead)
- The `delete` tool can accept an optional named Function to delete, in which case the path is not necessary (no named parameter indicates 'delete the Function in my cwd')

## Deployment Behavior

- **FIRST deployment** (no previous deploy): Should carefully gather registry, builder settings
- **SUBSEQUENT deployments**: Can call "deploy" tool directly with no arguments (reuses config from func.yaml)
- **OVERRIDE specific settings**: Call "deploy" tool with specific flags (e.g., --builder pack, --registry docker.io/user)
  - Example: "deploy with pack builder" → call deploy tool with --builder pack only

## Tool Usage Guide

### General Rules

**CRITICAL:** Before invoking ANY tool, ALWAYS read its help resource first to understand parameters and usage:
- Before 'create' → Read `func://help/create`
- Before 'deploy' → Read `func://help/deploy`
- Before 'build' → Read `func://help/build`
- Before 'list' → Read `func://help/list`
- Before 'delete' → Read `func://help/delete`

The help text provides authoritative parameter information and usage context.

### create

- **FIRST:** Read `func://help/create` for authoritative usage information
- **BEFORE calling:** Read `func://languages` resource to get available languages
- **BEFORE calling:** Read `func://templates` resource to get available templates
- Ask user to choose from the ACTUAL available options (don't assume/guess)
- **REQUIRED parameters:**
  - `language` (from languages list)
  - `path` (directory where the Function will be created)
- **OPTIONAL parameters:**
  - `template` (from templates list, defaults to "http" if omitted)

### deploy

- **FIRST:** Read `func://help/deploy` for authoritative usage information
- **REQUIRED parameters:**
  - `path` (directory containing the Function to deploy)
- **FIRST deployment:** Also requires `registry` parameter (e.g., docker.io/username or ghcr.io/username)
- **SUBSEQUENT deployments:** Only path is required (reuses previous config from func.yaml)
- **Optional** `builder` parameter: "host" (default for go/python) or "pack" (default for node/typescript/rust/java)
- Check if func.yaml exists at path to determine if this is first or subsequent deployment

#### Understanding the Registry Parameter

A common challenge with users is determining the right value for "registry". This is composed of two parts:

1. **Registry domain:** docker.io, ghcr.io, localhost:50000
2. **Registry user:** alice, func, etc.

When combined this constitutes a full "registry" location for the Function's built image.

**Examples:**
- `docker.io/alice`
- `localhost:50000/func`

The final Function image will then have the Function name as a suffix along with the :latest tag (example: `docker.io/alice/myfunc:latest`), but this is hidden from the user unless they want to fully override this behavior and supply their own custom value for the image parameter.

**Important guidance:**
- It is important to carefully guide the user through the creation of this registry argument, as this is often the most challenging part of getting a Function deployed the first time
- Ask for the registry. If they only provide the DOMAIN part (eg docker.io or localhost:50000), ask them to either confirm there is no registry user part or provide it
- The final value is the two concatenated with a forward slash
- Subsequent deployments automatically reuse the last setting, so this should only be asked for on those first deployments
- BE SURE to verify the final format of this value as consisting of both a DOMAIN part and a USER part
- Domain-only is technically allowed, but should be explicitly acknowledged, as this is an edge case

#### First-time Deployment Considerations

A first-time deploy can be detected by checking the func.yaml for a value in the "deploy" section which denotes the settings used in the last deployment. If this is the first deployment:

- A user should be warned to confirm their target cluster and namespace is the intended destination (this can also be determined for the user using kubectl if they agree)
- The "builder" argument should be defaulted to "host" for Go and Python functions
- For other languages, the user should be warned that first-time builds can be slow because the builder images will need to be downloaded, and they must have Podman or Docker available

### build

- **FIRST:** Read `func://help/build` for authoritative usage information
- **REQUIRED parameters:**
  - `path` (directory containing the Function to build)
- Builds the container image without deploying
- Useful for creating custom deployments using .yaml files or integrating with other systems which expect containers
- Uses same builder settings as deploy would use
- The user should be notified this is an unnecessary step if they intend to deploy, as building is handled as part of deployment

### list

- **FIRST:** Read `func://help/list` for authoritative usage information
- Does NOT use path parameter (operates on cluster, not local files)
- Optional `namespace` parameter to list Functions in specific namespace
- Returns list of deployed Functions in current/specified namespace

### delete

- **FIRST:** Read `func://help/delete` for authoritative usage information
- Supports TWO modes (mutually exclusive):
  1. **Delete by PATH:** Provide 'path' parameter (reads function name from func.yaml at that path)
  2. **Delete by NAME:** Provide 'name' parameter (deletes named function from cluster)
- Exactly ONE of 'path' or 'name' must be provided, not both
- Deleting does not affect local files (source). Only cluster resources.

### config_volumes, config_labels, config_envs

- All config tools require the 'path' parameter
- path points to the Function directory whose func.yaml will be modified
- These tools modify local func.yaml files only (changes take effect on next deploy)
