# work-preview

`work-preview` gives same-host development servers temporary HTTPS hostnames. A local gRPC service records previews in MySQL, writes one Caddy site file per preview, and gracefully reloads a parent Caddyfile. A preview expires after one hour without HTTP traffic.

The parent Caddyfile must contain:

```caddyfile
import /run/work-preview/caddy/*.caddy
```

## Usage

```sh
work-preview expose --port 3000
work-preview list
work-preview delete <preview-id>
```

The NixOS module is available as `nixosModules.default`. Its Cloudflare origin certificate and key paths stay outside the Nix store.

## Development

```sh
nix develop
go test ./...
nix flake check
```
