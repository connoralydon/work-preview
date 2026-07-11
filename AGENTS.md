# Preview Agent Instructions

## Expose a dev server

1. Start the dev server on an unused loopback port. Never bind it publicly.
2. Run `work-preview expose --port <port> --json`.
3. Report the returned HTTPS URL and retain the returned preview ID.
4. Delete it with `work-preview delete <id>` when finished. Otherwise it expires after one hour without HTTP traffic. Use `--until-reboot` only when forwarding must remain open without traffic; it is closed when the machine reboots.

The default prefix is `<short-commit>-<branch>-<repo>`, with random hexadecimal fallback outside Git. Use `--prefix <dns-label>` only to override it. Do not edit generated files under `/run/work-preview/caddy`.

## Work on this repository

- Enter the toolchain with `nix develop`.
- Generate protobuf code with `go generate ./...`.
- Run `go test ./...` and `nix flake check` before finishing.
- Keep SQLite as the source of truth and write Caddy snippets atomically.
- Add schema changes as contiguous numbered files under `internal/preview/migrations/`.
