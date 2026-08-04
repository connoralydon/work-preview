# Preview Agent Instructions

## Expose a dev server

`work-preview` does not serve project content. It exposes a development server that is already running on the local machine.

1. Find the project's normal development command or build its static content.
2. Start the server on an unused loopback port and keep it running. Bind it to `127.0.0.1`, not a public interface.
3. Confirm that `http://127.0.0.1:<port>` responds before you expose it.
4. Run `work-preview expose --port <port> --json`. Add `--public` only when the user explicitly requests a public preview.
5. Report the returned `url` and retain the returned `id`.
6. Delete it with `work-preview delete <id>` when finished. Otherwise it expires after one hour without HTTP traffic.

For example, start and expose a typical project development server:

```sh
npm run dev -- --host 127.0.0.1 --port 4173 > /tmp/work-preview-dev.log 2>&1 &
curl --fail --retry 20 --retry-connrefused --retry-delay 1 http://127.0.0.1:4173/ >/dev/null
work-preview expose --port 4173 --json
```

To serve built static content instead:

```sh
python3 -m http.server 4173 --bind 127.0.0.1 --directory dist > /tmp/work-preview-static.log 2>&1 &
curl --fail --retry 20 --retry-connrefused --retry-delay 1 http://127.0.0.1:4173/ >/dev/null
work-preview expose --port 4173 --json
```

The JSON result has the form `{"id":"<preview-id>","url":"https://<preview-host>"}`. For an explicitly requested public preview, use `work-preview expose --port 4173 --public --json`. Remove a preview with `work-preview delete <preview-id>`.

The default prefix is `<short-commit>-<branch>-<repo>`, with random hexadecimal fallback outside Git. Public previews always use a random prefix and omit Git metadata. Use `--prefix <dns-label>` only to override it. Do not edit generated files under `/run/work-preview/caddy`.

## Work on this repository

- Enter the toolchain with `nix develop`.
- Generate protobuf code with `go generate ./...`.
- Run `work-preview-test` inside `nix develop` and `nix flake check` before finishing.
- After editing and testing, commit and push the changes.
- Keep SQLite as the source of truth and write Caddy snippets atomically.
- Add schema changes as contiguous numbered files under `internal/preview/migrations/`.
