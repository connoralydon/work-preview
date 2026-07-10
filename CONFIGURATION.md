# Config Repository Integration

Use `work-preview` as a flake input and import its NixOS module into the host that runs Caddy and agent development servers.

## Flake Input

```nix
{
  inputs.work-preview = {
    url = "git+ssh://git@git.boringbison.xyz:2222/connoralydon/work-preview.git";
    inputs.nixpkgs.follows = "nixpkgs";
  };
}
```

The user updating the config repository lock file must have SSH access to `git.boringbison.xyz:2222`. Commit the resulting `flake.lock` change so deployments use a pinned revision.

Add the module to the appropriate `nixosSystem`:

```nix
modules = [
  inputs.work-preview.nixosModules.default
  ./hosts/preview-host.nix
];
```

## Host Module

```nix
{ ... }:
{
  services.caddy = {
    enable = true;
    extraConfig = ''
      import /run/work-preview/caddy/*.caddy
    '';
  };

  services.work-preview = {
    enable = true;
    domain = "p.boringbison.xyz";
    groupMembers = [ "agent-user" ];
  };
}
```

`groupMembers` controls access to the local gRPC socket. Add every local user that will run `work-preview expose`. The module installs the CLI system-wide.

The defaults assume the standard NixOS Caddy service and its generated root config at `/etc/caddy/caddy_config`. Set `rootCaddyfile` or `caddyAdminAddress` only when the parent Caddy deployment differs.

## Infrastructure

- Point `*.p.boringbison.xyz` at the host running Caddy.
- Keep development servers on `127.0.0.1`; only Caddy should accept public traffic.
- Let the parent Caddy deployment handle TLS and ports 80/443.
- Do not import `/run/work-preview/caddy/*.caddy` from more than one Caddy instance.
- The SQLite database is fixed at `/var/lib/work-preview/work-preview.db` and is managed by the service state directory.

## Deploy And Verify

Build the host before activation using the config repository's normal deployment workflow. After activation:

```sh
systemctl status work-preview
work-preview expose --port 3000 --json
work-preview list
work-preview delete <preview-id>
```

Confirm that Caddy reloads successfully and that the generated URL reaches a server bound to the exposed loopback port.

Update the service through the config repository lock file:

```sh
nix flake lock --update-input work-preview
nix flake check
```
