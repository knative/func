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
| registry  | {{if .Registry}}`{{.Registry}}`{{else}}**not provided** — ask the user in step 5{{end}} |
| cluster   | `{{.Cluster}}`{{if eq .Cluster "local"}} (a local cluster, e.g. kind){{else}} (a remote/shared cluster){{end}} |

Treat provided values as decided: do not re-ask for them. Ask only for the
values marked "not provided".
{{if .Readonly}}
> **This server is in read-only mode.** The registry, deploy and remote-invoke
> steps mutate cluster state and would be refused, so they are omitted below.
> Run steps 1 through 4, give the step 8 summary for what was accomplished
> locally, and tell the user that finishing onboarding — deploying and
> invoking a live instance — requires restarting the MCP server with
> `FUNC_ENABLE_MCP_WRITE=true`.
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

{{if .Language}}The language is already chosen: `{{.Language}}`. Read the
`func://languages` resource anyway and verify `{{.Language}}` is listed. If it
is not, tell the user and ask them to choose from the languages actually
available.{{else}}Read the `func://languages` resource and present the user
with the runtimes it actually reports. Do not offer a guessed or remembered
list — the available runtimes depend on the installed binary and any
configured template repositories. Ask the user to choose one and wait for
their answer.{{end}}

## Step 3 — Scaffold

Ask the user where the Function should live, and what it should be called if
the directory name is not the name they want. Then call the `create` tool
with:

- `language`: the runtime from step 2
- `template`: `{{.Template}}`
- `path`: the **absolute** path to the Function directory

Then `cd` into that directory yourself and stay there for the rest of this
session — every later tool call takes the same absolute path. Briefly show
the user what was generated (the handler file and `func.yaml`) so they know
where their code lives.

## Step 4 — Local run and invoke

Prove the Function works before involving a registry or a cluster.

1. Call `run` with the Function's absolute `path`. It builds if needed, starts
   the Function, and returns a `pid` and a `url`.
2. Call `invoke` with the same `path` and `target: "local"`. With no `data`, a
   real `{"message":"Hello World"}` payload is sent — the handler actually
   runs.
3. Show the user the response body verbatim. This is the moment the Function
   becomes real to them; do not paraphrase it.
4. Call `run_stop` with the same `path`. Always do this, including when the
   invoke failed, so the port and process are released.

Warn the user that the first local build{{if .Language}} for
`{{.Language}}`{{end}} can be slow: builder images may need to be
downloaded, and Podman or Docker must be available.
{{if not .Readonly}}
## Step 5 — Registry configuration

{{if .Registry}}Use the registry `{{.Registry}}`. Confirm it is well-formed
before continuing: a registry value is a **domain** plus a
**user/organization** joined by a slash, such as `docker.io/alice`. If it is
domain-only, acknowledge that edge case explicitly with the user. Also
confirm they are logged in to it (`docker login` / `podman login`), since the
deploy in step 6 pushes there.{{else}}Ask the user for their container registry. This is
the single most common place first-time onboarding goes wrong, so guide them
concretely rather than just asking:

- A registry value is a **domain** plus a **user/organization**, joined by a
  slash: `docker.io/alice`, `ghcr.io/alice`, `quay.io/alice`.
- If they give only a domain (`docker.io`), ask them to supply the user part,
  or to explicitly confirm they mean the domain-only form.
- The image name is derived automatically as `<registry>/<function>:latest`;
  they do not need to supply an image name.
- They must be logged in to that registry (`docker login` / `podman login`),
  because the deploy in step 6 pushes to it.
{{if eq .Cluster "local"}}- On a local cluster, an in-cluster registry such as
  `registry.localtest.me/func` avoids pushing over the network entirely, if
  their setup provides one.
{{end}}
Wait for the user's answer, then echo the final value back and confirm it
before continuing.{{end}}

## Step 6 — Deploy

Call the `deploy` tool with:

- `path`: the Function's absolute path
- `registry`: the value from step 5{{if eq .Language "go"}}
- `builder`: `host` (the fastest builder for Go){{else if eq .Language "python"}}
- `builder`: `host` (the fastest builder for Python){{else if not .Language}}
- `builder`: `host` for Go and Python; omit it for other runtimes so the
  default (`pack`) is used{{end}}

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
- **Live URL** {{if .Readonly}}(not applicable — no deploy was performed in read-only mode){{else}}(from `describe` in step 6){{end}}
- **Runtime** (language and template)
- **Registry**
- **Namespace** {{if .Readonly}}(not applicable){{else}}(from `describe` in step 6){{end}}

Take every value from actual tool output, not from what was requested — the
point of the summary is to tell the user what is true, not what was intended.
Then tell them the two things they will do next most often: edit the handler
and re-run `deploy` (which reuses the settings now stored in `func.yaml`, so
no arguments beyond `path` are needed).
