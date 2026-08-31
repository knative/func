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
- `invoke` tool: path = absolute path to Function directory (where func.yaml exists)
- `config_*_list`, `config_*_add`, `config_*_remove` tools: path = absolute path to Function directory (where func.yaml exists)
- `run` tool: path = absolute path to Function directory (where func.yaml exists)
- `run_stop` tool: path = the SAME absolute path passed to `run` when starting it

**IMPORTANT:** You must use absolute paths (e.g., `/Users/name/myproject/myfunc`), NOT relative paths (e.g., `.` or `myfunc`). The MCP server process runs in a different directory than your current working directory, so relative paths will not resolve correctly.

**Exceptions:**
- The `list` tool operates on the cluster, not local files, so it does NOT use a path parameter (it uses namespace instead)
- The `delete`, `describe` and `logs` tools each require exactly one of `path` or `name`; they do NOT support a no-argument CWD mode (the MCP server process has its own working directory unrelated to the Function being managed)

## Deployment Behavior

- **FIRST deployment** (no previous deploy): Should carefully gather registry, builder settings
- **SUBSEQUENT deployments**: Can call "deploy" tool directly with no arguments (reuses config from func.yaml)
- **OVERRIDE specific settings**: Call "deploy" tool with specific flags (e.g., --builder pack, --registry docker.io/user)
  - Example: "deploy with pack builder" → call deploy tool with --builder pack only

## Prompts

This server also exposes prompts: named, parameterized workflows the client
invokes on the user's behalf.

### onboard

- Drives full end-to-end onboarding: prerequisites → language → scaffold →
  registry → local run and invoke → deploy → remote invoke → summary
- The registry is gathered before the local run, because a containerized build
  has to name an image and so fails without one
- All four arguments (`language`, `template`, `registry`, `cluster`) are
  optional. Supplied values are treated as decided; omitted ones are gathered
  from the user as the relevant step is reached
- `template` defaults to `http` and `cluster` defaults to `local`. Only
  `cluster` is validated against a fixed set; `language` and `template` are
  passed through, since the valid values depend on the installed binary and on
  any added template repositories
- In read-only mode the deploy and remote-invoke steps are omitted from the
  returned prompt, since `deploy` would be refused
- If a user asks to "get started", "set up a Function from scratch", or
  similar, suggest they invoke this prompt rather than improvising the
  sequence yourself

## Tool Usage Guide

### General Rules

**CRITICAL:** Before invoking ANY tool, ALWAYS read its help resource first to understand parameters and usage:
- Before 'version' → Read `func://help/version`
- Before 'create' → Read `func://help/create`
- Before 'deploy' → Read `func://help/deploy`
- Before 'build' → Read `func://help/build`
- Before 'invoke' → Read `func://help/invoke`
- Before 'list' → Read `func://help/list`
- Before 'describe' → Read `func://help/describe`
- Before 'logs' → Read `func://help/logs`
- Before 'delete' → Read `func://help/delete`
- Before 'run' → Read `func://help/run`

The help text provides authoritative parameter information and usage context.

### version

- **FIRST:** Read `func://help/version` for authoritative usage information
- Takes no parameters
- Reports the version (and git commit hash, when available) of the func client binary being driven
- Use this to gate usage of newer tools/flags on the version of func actually installed, before assuming they are supported

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

1. **Registry domain:** docker.io, ghcr.io, registry.localtest.me
2. **Registry user:** alice, func, etc.

When combined this constitutes a full "registry" location for the Function's built image.

**Examples:**
- `docker.io/alice`
- `registry.localtest.me/func`

The final Function image will then have the Function name as a suffix along with the :latest tag (example: `docker.io/alice/myfunc:latest`), but this is hidden from the user unless they want to fully override this behavior and supply their own custom value for the image parameter.

**Important guidance:**
- It is important to carefully guide the user through the creation of this registry argument, as this is often the most challenging part of getting a Function deployed the first time
- Ask for the registry. If they only provide the DOMAIN part (eg docker.io or registry.localtest.me), ask them to either confirm there is no registry user part or provide it
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

### invoke

- **FIRST:** Read `func://help/invoke` for authoritative usage information
- Sends a test request to a running Function instance, either local or remote
- **REQUIRED parameters:**
  - `path` (directory containing the Function to invoke)
- **Optional** `target` parameter: "local", "remote", or a URL. Defaults to preferring a locally running instance over remote
- If `data` and `file` are both omitted, a real default payload is still sent: `{"message":"Hello World"}` with content-type `application/json`, source `/boson/fn`, type `boson.fn`, and method `POST`. This is NOT an empty/no-op probe — the Function's handler actually runs against this payload
- **If using `file`:** you MUST pass an ABSOLUTE path, just like `path`. `file` is resolved relative to the MCP server process's working directory (NOT your current working directory and NOT relative to the `path` parameter), so a relative value will often fail or read the wrong file
- **CAUTION:** Invoking a Function executes its handler and may trigger arbitrary, real side effects (e.g. sending an email, charging a payment, writing to a database) — this is especially true with `target: "remote"`, which hits the live deployed instance. Do not invoke automatically without considering whether the Function's handler is safe to run; when in doubt, confirm with the user first, particularly before invoking a remote/production instance
- An error is returned if the invocation fails (e.g. non-2xx HTTP response), so a successful call without an error confirms the Function responded correctly

### list

- **FIRST:** Read `func://help/list` for authoritative usage information
- Does NOT use path parameter (operates on cluster, not local files)
- Optional `namespace` parameter to list Functions in specific namespace
- Returns structured JSON: an `items` array (name, namespace, runtime, url, ready, deployer for each deployed Function; empty if none found) plus `warnings` for any non-fatal issues encountered while listing (e.g. a deployer backend that couldn't be reached)

### describe

- **FIRST:** Read `func://help/describe` for authoritative usage information
- Describes a **deployed** Function instance on the cluster; it never just reads local `func.yaml`. This tool will fail if the Function has not yet been deployed (e.g. calling it right after 'create' but before 'deploy' is a usage error, not a tool bug)
- Supports TWO modes (mutually exclusive):
  1. **Describe by PATH:** Provide 'path' parameter (reads the Function's name/namespace from func.yaml at that path, then describes whatever is deployed for that Function)
  2. **Describe by NAME:** Provide 'name' parameter (describes the named Function on the cluster)
- Exactly ONE of 'path' or 'name' must be provided, not both
- 'namespace' is only valid together with 'name'; providing both 'path' and 'namespace' is rejected, since path mode already determines the namespace from the Function's own deploy identity
- Read-only; does not modify local files or cluster resources

### logs

- **FIRST:** Read `func://help/logs` for authoritative usage information
- Exactly one of `path` or `name` is required (same shape as `delete` / `describe`) — never both, and never neither
- `path` must be an absolute path to the Function project directory (reads func.yaml); `name` is the deployed Function name on the cluster
- The Function must already be **deployed** — in path mode the tool still talks to the cluster via the project's deploy identity, so calling it before `deploy` is a usage error, not a tool bug
- Returns a **finite snapshot** of recent logs (it prints and exits; it is not a live stream); the default is the most recent 1000 lines per pod, so the output is bounded even when you pass neither `since` nor `tail`
- Narrow it further with `since` (time window, e.g. `30s`, `5m`, `2h`) and/or a smaller `tail` (most recent lines per pod); a negative `tail` means unlimited, and is the only way to ask for every retained line
- If the payload exceeds the tool's size limit, only the most recent lines are returned and `truncated` is set to true; re-run with `tail` or `since` rather than assuming the output is complete
- **Always read `warnings` before concluding a Function produced no output.** Empty or partial `logs` with a zero exit is normal and explained there: the Function may have scaled to zero (no pods, therefore no logs), or the logs of only some of its pods could be read
- `namespace` applies when identifying the Function by `name`. In `path` mode the namespace is read from the Function's own deploy identity in func.yaml and `namespace` has no effect — the same behavior as `func logs`, which this tool does not add rules on top of
- When more than one pod serves the Function, each line is prefixed with `[pod/<name>] `
- `follow`/streaming is deliberately not exposed: it never terminates, so it is unusable from an agent. Use `since`/`tail` and call again to observe new output
- Only Functions deployed with the default **knative** deployer are supported; for other deployers the CLI returns an error directing you to `kubectl logs`
- `verbose` output is written to the same stream the notices come back on, so enabling it makes `warnings` considerably noisier; the returned notices are bounded and older ones are dropped when they overflow
- This tool is **read-only** — it never modifies any state
- Use logs to diagnose a deployed Function after `deploy`, especially when combined with `invoke` to trigger the Function and observe its output

### delete

- **FIRST:** Read `func://help/delete` for authoritative usage information
- Supports TWO modes (mutually exclusive):
  1. **Delete by PATH:** Provide 'path' parameter (reads function name from func.yaml at that path)
  2. **Delete by NAME:** Provide 'name' parameter (deletes named function from cluster)
- Exactly ONE of 'path' or 'name' must be provided, not both
- Deleting does not affect local files (source). Only cluster resources.

### config_git_set, config_git_remove

- **BEFORE calling:** Read `func://help/config/git/set` or `func://help/config/git/remove`
- Both tools require the `path` parameter (absolute path to the Function directory)
- `config_git_set` — configures Git source repository settings for pipeline-based builds:
  - **REQUIRED:** `git_url` (repository URL) and `git_branch` (branch or tag, e.g. `main`)
  - **OPTIONAL:** `git_dir` (subdirectory in the repo; defaults to repository root when omitted)
  - **OPTIONAL:** `git_provider` (auto-detected from URL; override only if detection fails)
  - **OPTIONAL:** `config_local`, `config_cluster`, `config_remote` (boolean flags to control which pipeline resources are created; defaults to local-only when none are specified)
  - **OPTIONAL:** `gh_access_token` — required only when `config_remote` is true to create a GitHub webhook
  - Changes are written to `func.yaml` and take effect on the next pipeline build
- `config_git_remove` — removes Git settings and associated pipeline resources:
  - **OPTIONAL:** `delete_local` — removes local pipeline template files
  - **OPTIONAL:** `delete_cluster` — removes cluster credentials and pipeline resources
  - When neither flag is provided, local resources are removed by default
- **WARNING:** `config_git_remove` with `delete_cluster: true` is destructive — cluster pipeline resources are deleted permanently

### run

- **FIRST:** Read `func://help/run` for authoritative usage information
- **REQUIRED parameters:**
  - `path` (absolute path to the directory containing the Function to run; there is no CWD-based default — the MCP server's own working directory is unrelated to yours)
- **OPTIONAL parameters:**
  - `registry` (container registry; only needed if the Function's image must be named/built)
  - `build` (force a rebuild before running; omit to let func build automatically only when out of date)
  - `port` (host port to bind; omit to use 8080 or the first available port)
- Builds the Function locally if needed, starts it, and returns once it is up: `pid` (process ID) and `url`
- The Function keeps running in the background after the tool call returns
- Only ONE run may be active per Function path at a time — calling `run` again for a path that is already running fails with a clear error; call `run_stop` first
- ALWAYS call `run_stop` with the matching `path` when finished, to free the port and clean up
- Not a cluster operation, so unaffected by read-only mode

### run_stop

- Stops a Function previously started with `run`
- **REQUIRED:** `path` (absolute) must match the path used to start it with `run`
- Idempotent: calling `run_stop` for a path with no active run (already stopped, or never started) succeeds with an informational message rather than erroring
- Not a cluster operation, so unaffected by read-only mode

### config_envs_list, config_envs_add, config_envs_remove

- **BEFORE calling add/remove:** Consider reading `func://help/config/envs` for authoritative usage
- All three tools require the 'path' parameter (absolute path to the Function directory)
- `config_envs_list` — read-only; lists current environment variables
- `config_envs_add` — adds an environment variable; optional `name` and `value` parameters
- `config_envs_remove` — removes an environment variable by `name`
- add/remove modify local func.yaml only (changes take effect on next deploy)

### config_labels_list, config_labels_add, config_labels_remove

- **BEFORE calling add/remove:** Consider reading `func://help/config/labels` for authoritative usage
- All three tools require the 'path' parameter (absolute path to the Function directory)
- `config_labels_list` — read-only; lists current labels
- `config_labels_add` — adds a label; optional `name` and `value` parameters
- `config_labels_remove` — removes a label by `name`
- add/remove modify local func.yaml only (changes take effect on next deploy)

### config_volumes_list, config_volumes_add, config_volumes_remove

- **BEFORE calling add/remove:** Consider reading `func://help/config/volumes` for authoritative usage
- All three tools require the 'path' parameter (absolute path to the Function directory)
- `config_volumes_list` — read-only; lists current volume mounts
- `config_volumes_add` — adds a volume; accepts `type` (configmap, secret, pvc, emptydir), `mountPath`, `source`, `medium`, `size`, `readOnly`
- `config_volumes_remove` — removes a volume by `mountPath`
- add/remove modify local func.yaml only (changes take effect on next deploy)

### config_ci

- **BEFORE calling:** Consider reading `func://help/config/ci` for authoritative usage
- Requires the `path` parameter (absolute path to the Function directory)
- Generates a GitHub Actions workflow (`.github/workflows/func-deploy.yaml` by default) that builds, tests, and deploys the Function on every push to `branch`
- This is the last step of the production lifecycle: create → build/deploy → **config_ci** to wire up continuous deployment
- Fails if a workflow file already exists at that path unless `force` is set to `true`
- **IMPORTANT:** This command is gated behind an experimental feature flag. If it fails with an error like `unknown command "ci" for "config"`, the `func` binary backing this MCP server does not have `FUNC_ENABLE_CI_CONFIG=true` set in its environment — inform the user they need to set that environment variable when starting the MCP server (e.g. in their MCP client's server config) and restart it
