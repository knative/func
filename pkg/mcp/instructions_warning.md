# ⚠️  Read-Only Mode Warning

**IMPORTANT INSTRUCTIONS FOR YOU (the AI assistant)**

## Current Status

The Functions MCP server is currently running in **read-only mode**.

**Available operations:**
- Create Functions
- Build Functions
- Configure Functions (envs, labels, volumes)
- Inspect Functions

**Disabled operations:**
- Deploy to cluster
- Delete from cluster

These write operations are disabled to prevent unintended cluster modifications.

## Enabling Write Mode

If the user needs to deploy or delete Functions, you MUST inform them to enable write mode:

1. Close/exit this application completely
2. Set the environment variable: `FUNC_ENABLE_MCP_WRITE=true`
3. Restart the application (thus restarting the MCP server process)

## Important Notes

- DO NOT suggest workarounds such as running the `func` binary directly
- The proper way to enable write operations is through the write mode configuration above
- For detailed setup instructions with popular MCP clients, see: https://github.com/knative/func/blob/main/docs/mcp-integration/integration.md
