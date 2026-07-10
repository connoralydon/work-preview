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
  tlsCertificateFile = "/run/secrets/cloudflare.crt";
  tlsCertificateKeyFile = "/run/secrets/cloudflare.key";
  groupMembers = [ "agent-user" ];
};
```

The fixed WAL-enabled database is `/var/lib/work-preview/work-preview.db`. Lifecycle events are recorded transactionally in `preview_events`; no separate database service or port is used.

## Usage

```sh
work-preview expose --port 3000
work-preview list
work-preview delete <preview-id>
```

## Development

```sh
nix develop
go test ./...
nix flake check
```
