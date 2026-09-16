# Set up odeduck

Read this only when the binary or agent connection is missing. Preserve working installations and existing
MCP configuration. The runtime and catalogue are installed separately from this skill.

## 1. Install the binary and catalogue

macOS / Linux:

```sh
curl -fsSL https://github.com/JungHoonGhae/odeduck/releases/latest/download/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://github.com/JungHoonGhae/odeduck/releases/latest/download/install.ps1 | iex
```

Verify installation and a first search without logging in:

```sh
odeduck version
odeduck catalog search "건축물" --limit 5 --semantic=false -f json
```

The URLs follow the latest stable release; the installers resolve a concrete version and verify downloaded
assets against its checksums. Preserve an existing version pin when the user requires reproducibility.
For upgrades, inspect the installed version and release notes first, then rerun the installer within the
authorized setup scope. Reconnect MCP afterward and re-read its guide; updating the binary does not replace
an already running server. Neither the binary nor this `skills.sh` installation updates automatically.

A missing binary, missing catalogue and zero matching records are different outcomes. Report the actual
error; do not treat an installation failure as an empty catalogue. No semantic index is required here.

## 2. Connect the user's agent, if MCP is wanted

Use only the command for the selected host:

```sh
# Codex
codex mcp add odeduck -- odeduck mcp

# Claude Code
claude mcp add odeduck -- odeduck mcp

# Gemini CLI
gemini mcp add --scope user odeduck odeduck mcp
```

For Cursor or Claude Desktop, merge this entry into the host's existing MCP configuration:

```json
{
  "mcpServers": {
    "odeduck": {
      "command": "odeduck",
      "args": ["mcp"]
    }
  }
}
```

Start a new agent conversation or restart the host if the tools are not visible. Verify that the odeduck
tools are listed before claiming MCP is connected. CLI use remains available without MCP.

## 3. Log in when an authenticated operation needs it

Have the user run `odeduck login` and complete government SSO in the opened browser. If the current task
already authorizes starting login, open it for them, then wait for the human step. Never automate SSO or
extract credentials for model context. Resume the selected API's access check and call after login.

Sessions and keys stay in local runtime storage. MCP results are sent to the host AI. Provider-scoped keys
for external LINK APIs use separate local setup as reported by inspection.

## Update an installed skill

For a GitHub-installed skill, read the manager's current `--help`, preserve local changes, and use the
same installation scope. For a project installation, run this from that project:

```sh
npx skills@latest update odeduck --project
```

For a global installation, replace `--project` with `--global`. This updates `odeduck`, not the Go binary.
A local-path installation is not tracked
as a remote release: reinstall from the updated local source. Preserve intentional local customizations
and version pins. Start a new agent session after skill updates so previously loaded instructions are refreshed.
For the binary, use the stable installer above and reconnect MCP. Plugin-managed auto-updates would be a
separate distribution mode; the current skill installation does not register a background update hook.
