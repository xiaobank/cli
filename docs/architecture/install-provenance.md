# Install Provenance

Released Entire CLI binaries may include installer-owned provenance at:

- `~/.config/entire/install.json`

The CLI treats this file as read-only. Installers are responsible for creating
and updating it. The CLI may fall back to legacy heuristics when the file is
missing for backwards compatibility.

## File format

Example:

```json
{
  "manager": "brew",
  "channel": "nightly",
  "package": "entire@nightly",
  "installed_at": "2026-04-11T12:00:00Z"
}
```

Fields:

- `manager`: installer identity such as `install.sh`, `brew`, or `scoop`
- `channel`: release channel such as `stable` or `nightly`
- `package`: installer-native package identifier used for update commands
- `installed_at`: RFC3339 UTC timestamp written by the installer

## Update mapping

The CLI derives update hints from provenance:

- `install.sh` + `stable` -> `curl -fsSL https://entire.io/install.sh | bash`
- `brew` + `stable|nightly` -> `brew upgrade <package>`
- `scoop` + `stable|nightly` -> `scoop update <package>`

If provenance is absent, the CLI may infer a legacy update hint from the
executable path.
