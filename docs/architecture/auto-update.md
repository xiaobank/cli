# Auto-Update

The Entire CLI can offer or install new releases automatically after its daily
version check. The feature is **off by default** and opt-in.

## Configuration

Settings live at `~/.config/entire/settings.json` (machine-wide, not
per-repository):

```json
{
  "auto_update": "prompt"
}
```

Values:

- `off` (default) — show the notification only; no prompt, no execution.
- `prompt` — after the notification, ask the user to confirm and run the installer on yes.
- `auto` — install silently when guardrails pass (see below).

### CLI subcommands

```
entire auto-update status              # print current mode + config path
entire auto-update enable              # set mode to "prompt"
entire auto-update disable             # set mode to "off"
entire auto-update set <off|prompt|auto>
```

### Manual update

```
entire update              # resolve installer, prompt Y/N, run it
entire update --yes        # skip confirmation
entire update --check-only # print the installer command without running it
```

## Guardrails

The auto-update path refuses to execute unless all of the following hold:

- `auto_update` is `prompt` or `auto`.
- stdout is a terminal.
- `CI` environment variable is empty.
- `ENTIRE_NO_AUTO_UPDATE` environment variable is empty (kill switch).

Additionally, `auto` mode requires:

- An `InstallProvenance` file at `~/.config/entire/install.json` resolves to a
  known installer (brew / scoop / install.sh). The executable-path fallback is
  not trusted for silent installs.
- The release was published at least 24 hours ago (soak delay — cushion
  against a bad release).
- No failed attempt was recorded in the last 24 hours.

## Installer mapping

Derived from `InstallProvenance` (see `docs/architecture/install-provenance.md`):

- `install.sh` + `stable` → `curl -fsSL https://entire.io/install.sh | bash`
- `brew` + `stable|nightly` → `brew upgrade <package>`
- `scoop` + `stable|nightly` → `scoop update <package>`

The installer is run via `sh -c` on Unix and `cmd /C` on Windows. stdin,
stdout, and stderr are wired to the user so passwords and progress output
flow through.

## Failure handling

Failed attempts are recorded in `~/.config/entire/version_check.json`.
In `auto` mode the CLI backs off for 24 hours after a failure; `prompt` mode
always re-prompts on the next run.
