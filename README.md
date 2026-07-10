# work-preview

`work-preview` gives loopback development servers temporary HTTPS hostnames. It stores previews in embedded SQLite, writes atomic Caddy site files, and expires previews after one hour without HTTP traffic.

## NixOS

Import the generated sites from the parent Caddyfile:

```caddyfile
import /run/work-preview/caddy/*.caddy
```

Then import this flake's `nixosModules.default` and configure:

```nix
services.work-preview = {
  enable = true;
  groupMembers = [ "agent-user" ];
};
```

The fixed WAL-enabled database is `/var/lib/work-preview/work-preview.db`. Lifecycle events are recorded transactionally in `preview_events`; numbered embedded migrations are applied transactionally at startup using SQLite `user_version`. No separate database service or port is used.

See [CONFIGURATION.md](CONFIGURATION.md) for flake input, host module, DNS, and deployment guidance for a Nix config repository.

## Usage

```sh
work-preview expose --port 3000
work-preview list
work-preview delete <preview-id>
```

When `--prefix` is omitted, `expose` derives `<short-commit>-<branch>-<repo>` from the current Git worktree and sanitizes it as a DNS label. Outside a Git repository, the CLI generates a random 12-character hexadecimal prefix. Pass `--prefix <name>` to override it.

## Development

```sh
nix develop
go test ./...
nix flake check
```
