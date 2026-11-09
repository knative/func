# MCP Integration Quick Usage

This document shows how to configure your MCP server with various clients. You do **not** need to run `func mcp start` manually — MCP clients launch it themselves when configured.

Once configured, you can also use the MCP server **directly in agent mode while chatting with the model**. This enables seamless access to your functions and tools during conversations.

Base config to add:

```json
{
  "mcpServers": {
    "func-mcp": {
      "command": "func",
      "args": ["mcp", "start"]
    }
  }
}
```

This block should either be the full contents of your MCP config file (for clients that use a dedicated file) or merged into an existing `mcpServers` object. VS Code is the exception: it uses a different format, described in its section below.

---

## Read-Only and Write Mode

By default the MCP server runs in **read-only mode**. In this mode an agent can create, build (without pushing the image), run locally, and inspect Functions, and list template repositories. Anything that mutates configuration, the cluster, or the template repositories is refused, including:

* deploying and deleting Functions
* adding or removing environment variables, labels and volumes
* configuring Git source settings and CI workflows
* pushing a built image to a registry
* adding, renaming or removing template repositories

To allow these operations, set the environment variable `FUNC_ENABLE_MCP_WRITE=true` for the MCP server process, and restart the MCP server (usually by restarting the client). Set it in the client's MCP server config rather than in your shell, because the client launches the server. Most clients accept an `env` object next to `command` and `args`:

```json
{
  "mcpServers": {
    "func-mcp": {
      "command": "func",
      "args": ["mcp", "start"],
      "env": {
        "FUNC_ENABLE_MCP_WRITE": "true"
      }
    }
  }
}
```

Enabling write mode gives the agent the ability to create, modify, deploy and delete Functions on your currently connected cluster. Supervise it accordingly.

---

## Available Tools

The agent is given built-in instructions describing each tool, so you do not need to explain them. In summary:

| Area | Tools |
|------|-------|
| Server | `healthcheck`, `version` |
| Lifecycle | `create`, `build`, `deploy`, `delete`, `list`, `describe`, `logs`, `invoke` |
| Local run | `run`, `run_stop` |
| Configuration | `config_envs_*`, `config_labels_*`, `config_volumes_*`, `config_git_set`, `config_git_remove`, `config_ci` |
| Template repositories | `repository_list`, `repository_add`, `repository_rename`, `repository_remove` |

`config_ci` additionally requires `FUNC_ENABLE_CI_CONFIG=true` to be set for the server, as it is an experimental feature.

The server also provides an `onboard` prompt, which walks through creating, running and deploying a first Function, and help resources (`func://help/...`) for the underlying commands.

---

## Configuration Scope

* **Global configuration**: applies to all projects in the client. Usually stored in a global settings file under your home directory.
* **Local/project configuration**: applies only to the current project or workspace. Usually stored in the project folder.

If both exist, local/project config usually overrides global config.

---

## Claude Desktop

**Config file location:**

* macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
* Windows: `%APPDATA%/Claude/claude_desktop_config.json`
* Linux: `~/.config/Claude/claude_desktop_config.json`

**How to edit:**

* Open the file in a text editor.
* Replace contents with the base config above, or merge it into an existing `mcpServers` block.

Restart Claude Desktop after editing.

---

## Cursor

**Config file location:**

* Global: `~/.cursor/mcp.json`
* Local: `.cursor/mcp.json` inside the project folder

**How to edit:**

* Open or create the file.
* Use the base config above directly, or merge into an existing `mcpServers` object.

Restart Cursor after editing.

---

## Windsurf (Cascade)

**Config file location:**

* Linux/macOS: `~/.codeium/windsurf/mcp_config.json`
* Windows: `%APPDATA%/Codeium/Windsurf/mcp_config.json`

**How to edit:**

* Open the file in a text editor.
* Replace contents with the base config above, or merge into existing config.

Restart Windsurf after editing.

---

## VS Code

VS Code configures MCP servers in an `mcp.json` file (not under `mcpServers` in `settings.json`), and its format differs from the base config above: servers go under a top-level `servers` key instead of `mcpServers`, and declare a `type`.

**Config file location:**

* Local: `.vscode/mcp.json` inside the project folder (run **MCP: Open Workspace Folder MCP Configuration**)
* Global: the `mcp.json` in your VS Code user profile (run **MCP: Open User Configuration**)

**How to edit:**

* Open the file with one of the commands above (from the Command Palette), or create `.vscode/mcp.json`.
* Add the server under `servers`.

For example:

```json
{
  "servers": {
    "func-mcp": {
      "type": "stdio",
      "command": "func",
      "args": ["mcp", "start"]
    }
  }
}
```

To enable write mode, add `"env": { "FUNC_ENABLE_MCP_WRITE": "true" }` to the server entry.

Alternatively, run **MCP: Add Server** and choose **Command (stdio)**, or add it from a terminal:

```bash
code --add-mcp '{"name":"func-mcp","type":"stdio","command":"func","args":["mcp","start"]}'
```

VS Code starts the server when it is needed; use **MCP: List Servers** to start, stop or inspect it. See the [VS Code MCP documentation](https://code.visualstudio.com/docs/copilot/customization/mcp-servers) for the current details, as this area is changing.

---

## Claude Code

Add the server with the Claude Code CLI:

```bash
claude mcp add func func mcp start
```

To enable write mode, pass the environment variable when adding it:

```bash
claude mcp add func --env FUNC_ENABLE_MCP_WRITE=true -- func mcp start
```

Use `claude mcp list` to confirm it is registered.

---

## Quick Troubleshooting

* Ensure `func` is installed and in PATH.
* Use an absolute path if needed (e.g., `/usr/local/bin/func`).
* Restart the client after editing config files.
* If local and global configs conflict, the local one usually takes priority.

---

