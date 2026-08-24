# Full Function Onboarding

Take the user end-to-end: from nothing, to a scaffolded Function, to a
Function running locally, to a Function deployed and answering on a live URL.

Work through the steps below **in order**. Complete each one and report its
outcome to the user before starting the next. If a step fails, stop, explain
the failure in plain terms, and help the user resolve it rather than skipping
ahead — every later step depends on the ones before it.

## Session parameters

| Parameter | Value |
|-----------|-------|
| language  | {{if .Language}}`{{.Language}}`{{else}}**not provided** — ask the user in step 2{{end}} |
| template  | `{{.Template}}` |
| registry  | {{if .Registry}}`{{.Registry}}`{{else}}**not provided** — ask the user in step 4{{end}} |
| cluster   | `{{.Cluster}}`{{if eq .Cluster "local"}} (a local cluster, e.g. kind){{else}} (a remote/shared cluster){{end}} |

Treat provided values as decided: do not re-ask for them. Ask only for the
values marked "not provided".
{{if .Readonly}}
> **This server is in read-only mode.** The `deploy` tool is refused, so the
> deploy (step 6) and the remote invoke which depends on it (step 7) are
> omitted below. Run steps 1 through 5, give the step 8 summary for what was
> accomplished locally, and tell the user that finishing onboarding —
> deploying and invoking a live instance — requires restarting the MCP server
> with `FUNC_ENABLE_MCP_WRITE=true`. The registry in step 4 is still needed:
> the local build in step 5 has to name an image, even though read-only mode
> means nothing is pushed.
{{end}}
## Step 1 — Prerequisites

Call the `version` tool. It reports the version of the `func` binary this
server drives.

- If it succeeds, tell the user which version they are on and continue.
- If it fails because `func` is not installed or not on `PATH`, stop and guide
  the user through installing it (https://knative.dev/docs/functions/install-func/),
  then re-run this step. Do not attempt any later step until `version`
  succeeds.
{{if eq .Cluster "local"}}
Because `cluster` is `local`, also confirm with the user that a local cluster
(e.g. kind) is running and that `kubectl` points at it. `func` deploys to
whatever context `kubectl` is currently using, so a wrong context is the most
common cause of a surprising deployment target.
{{else}}
Because `cluster` is `remote`, confirm with the user which cluster and
namespace `kubectl` is currently pointed at, and that it is the intended
deployment target. Do this now, before anything is built.
{{end}}
## Step 2 — Language selection
{{if .Language}}
The language is already chosen. Read the `func://languages` resource anyway
and verify the chosen runtime is listed. If it is not, tell the user and ask
them to choose from the languages actually available — the runtimes on offer
depend on the installed binary and any configured template repositories.
{{else}}
Read the `func://languages` resource and present the user with the runtimes it
actually reports. Do not offer a guessed or remembered list — the available
runtimes depend on the installed binary and any configured template
repositories. Ask the user to choose one and wait for their answer.
{{end}}
## Step 3 — Scaffold

Ask the user where the Function should live. The Function's **name is the
basename of that directory** — there is no separate name argument — so if they
want a particular name, the directory has to carry it. Then call the `create`
tool with:

- `language`: the runtime from step 2
- `template`: `{{.Template}}`
- `path`: the **absolute** path to the Function directory

Every later tool call takes that same absolute path: no tool here has a
working-directory default, and the MCP server's own working directory is
unrelated to yours. If `create` rejects the template, read the
`func://templates` resource to see what the chosen runtime actually ships and
agree on one with the user.

Briefly show the user what was generated (the handler file and `func.yaml`) so
they know where their code lives.

## Step 4 — Registry configuration

This comes before the local run on purpose. The default builder (`pack`)
builds a container image, and naming that image requires a registry, so a
local run fails with "registry required" without one.
{{if .Readonly}}
Nothing is pushed to it in read-only mode: it is needed only to name the image
the local build produces.
{{else}}
Nothing is pushed to it until the deploy in step 6.
{{end}}{{if .Registry}}
Use the registry from the session parameters. Confirm it is well-formed before
continuing: a registry value is a **domain** plus a **user/organization**
joined by a slash, such as `docker.io/alice`. If it is domain-only,
acknowledge that edge case explicitly with the user.
{{else}}
Ask the user for their container registry. This is the single most common place
first-time onboarding goes wrong, so guide them concretely rather than just
asking:

- A registry value is a **domain** plus a **user/organization**, joined by a
  slash: `docker.io/alice`, `ghcr.io/alice`, `quay.io/alice`.
- If they give only a domain (`docker.io`), ask them to supply the user part,
  or to explicitly confirm they mean the domain-only form.
- The image name is derived automatically as `<registry>/<function>:latest`;
  they do not need to supply an image name.
{{if eq .Cluster "local"}}- On a local cluster, an in-cluster registry like `registry.localtest.me/func`
  avoids pushing over the network entirely, if their setup provides one.
{{end}}
Wait for the user's answer, then echo the final value back and confirm it
before continuing.
{{end}}{{if not .Readonly}}
Also confirm they are logged in to that registry (`docker login` /
`podman login`), since the deploy in step 6 pushes there.
{{end}}
## Step 5 — Local run and invoke

Prove the Function works locally before involving a cluster.

1. Call `run` with the Function's absolute `path` and the `registry` from step
   4. It builds if needed, starts the Function, and returns a `pid` and a
   `url`.
2. Call `invoke` with the same `path` and `target: "local"`. With no `data`, a
   real `{"message":"Hello World"}` payload is sent — the handler actually
   runs.
3. Show the user the response body verbatim. This is the moment the Function
   becomes real to them; do not paraphrase it.
4. Call `run_stop` with the same `path`. Always do this, including when the
   invoke failed, so the port and process are released.

Warn the user that the first local build can be slow: builder images may need
to be downloaded, and Podman or Docker must be available.
{{if not .Readonly}}
## Step 6 — Deploy

Call the `deploy` tool with:

- `path`: the Function's absolute path
- `registry`: the value from step 4
{{if .HostBuilder}}- `builder`: `host` (faster than the default `pack` builder for this runtime)
{{else if not .Language}}- `builder`: omit it unless the runtime chosen in step 2 is supported by the
  `host` builder, which is faster where it applies. The default (`pack`) works
  for every runtime, and `deploy` reports plainly when `host` does not support
  the runtime
{{end}}
This builds the image, pushes it to the registry, and creates the Function on
the cluster. It is the slowest step; tell the user what is happening rather
than going quiet.

On success, call `describe` with the same `path` and read `url` out of the
result. That URL — not anything parsed out of the deploy log — is the
authoritative live address. Also note `namespace` and `ready` from the same
result.

If `ready` is not `true`, do not declare success: report what `describe`
returned and help the user diagnose it.

## Step 7 — Remote invoke

Call `invoke` with the Function's `path` and `target: "remote"`. This hits the
deployed instance and runs its handler for real, so if the handler has been
modified to do anything with side effects, confirm with the user first.

Show the response verbatim. A successful call with no error is the proof that
onboarding worked end-to-end: the Function is deployed, routable, and
answering.
{{end}}
## Step 8 — Summary

Finish by printing a short, plain summary of what now exists:

- **Function name**
- **Live URL** {{if .Readonly}}(not applicable — read-only mode performed no deploy){{else}}(from `describe` in step 6){{end}}
- **Runtime** (language and template)
- **Registry** (the value from step 4)
- **Namespace** {{if .Readonly}}(not applicable — read-only mode performed no deploy){{else}}(from `describe` in step 6){{end}}

Take every value from actual tool output, not from what was requested — the
point of the summary is to tell the user what is true, not what was intended.
{{if .Readonly}}
Then tell them the two things they will do next most often: edit the handler
and re-run the local `run` and `invoke` from step 5. Deploying requires
restarting this server with `FUNC_ENABLE_MCP_WRITE=true`, after which the
settings now stored in `func.yaml` are reused and `deploy` needs no arguments
beyond `path`.
{{else}}
Then tell them the two things they will do next most often: edit the handler
and re-run `deploy` (which reuses the settings now stored in `func.yaml`, so
no arguments beyond `path` are needed).
{{end}}
