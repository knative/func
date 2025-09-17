# MCP Integration Quick Usage

This document shows how to configure your MCP server with various clients. You do **not** need to run `func mcp` manually — MCP clients launch it themselves when configured.

Once configured, you can also use the MCP server **directly in agent mode while chatting with the model**. This enables seamless access to your functions and tools during conversations.

Base config to add:

```json
{
  "mcpServers": {
    "func-mcp": {
      "command": "func",
      "args": ["mcp"]
    }
  }
}
```

This block should either be the full contents of your MCP config file (for clients that use a dedicated file) or merged into an existing config (for clients like VS Code).

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

**Config file location:**

* Global: `~/.config/Code/User/settings.json` (Linux/macOS)
* Global: `%APPDATA%/Code/User/settings.json` (Windows)
* Local: `.vscode/settings.json` inside the project folder

**How to edit:**

* VS Code’s `settings.json` is usually prefilled with many settings.
* Locate or add a top-level key `mcpServers`.
* Insert the base config block under it.

For example:

```json
{
  // other VS Code settings...
  "mcpServers": {
    "func-mcp": {
      "command": "func",
      "args": ["mcp"]
    }
  }
}
```

Restart VS Code after editing.

---

## Quick Troubleshooting

* Ensure `func` is installed and in PATH.
* Use an absolute path if needed (e.g., `/usr/local/bin/func`).
* Restart the client after editing config files.
* If local and global configs conflict, the local one usually takes priority.

---

