{self}: {
  config,
  lib,
  pkgs,
  ...
}: let
  cfg = config.services.work-preview;
  inherit (lib) mkEnableOption mkIf mkOption types;
in {
  options.services.work-preview = {
    enable = mkEnableOption "ephemeral development previews";

    package = mkOption {
      type = types.package;
      default = self.packages.${pkgs.system}.default;
      description = "work-preview package to run.";
    };

    domain = mkOption {
      type = types.str;
      default = "p.boringbison.xyz";
      description = "DNS suffix used for generated preview hostnames.";
    };

    rootCaddyfile = mkOption {
      type = types.str;
      default = "/etc/caddy/caddy_config";
      description = "Root Caddyfile containing the generated-snippet import.";
    };

    caddyAdminAddress = mkOption {
      type = types.str;
      default = "localhost:2019";
      description = "Caddy admin API address used for graceful reloads.";
    };

    snippetDirectory = mkOption {
      type = types.str;
      default = "/run/work-preview/caddy";
      readOnly = true;
      description = "Directory imported by the parent Caddyfile.";
    };

    logDirectory = mkOption {
      type = types.str;
      default = "/var/log/work-preview";
      description = "Directory containing per-preview Caddy access logs.";
    };

    ttl = mkOption {
      type = types.str;
      default = "1h";
      description = "Inactivity period after which a preview expires.";
    };

    sweepInterval = mkOption {
      type = types.str;
      default = "1m";
      description = "Interval for observing access logs and expiring previews.";
    };

    groupMembers = mkOption {
      type = types.listOf types.str;
      default = [];
      description = "Local users allowed to access the control socket.";
    };
  };

  config = mkIf cfg.enable {
    assertions = [
      {
        assertion = lib.hasPrefix "/" cfg.rootCaddyfile;
        message = "services.work-preview.rootCaddyfile must be an absolute path";
      }
    ];

    users.groups.work-preview.members = cfg.groupMembers;
    users.users.work-preview = {
      isSystemUser = true;
      group = "work-preview";
    };

    environment.systemPackages = [cfg.package];

    systemd.tmpfiles.rules = [
      "d ${cfg.logDirectory} 0770 caddy work-preview - -"
    ];

    systemd.services.work-preview = {
      description = "Ephemeral development preview controller";
      wantedBy = ["multi-user.target"];
      after = ["caddy.service"];
      serviceConfig = {
        User = "work-preview";
        Group = "work-preview";
        RuntimeDirectory = "work-preview";
        # Caddy must traverse this directory to read imported snippets. The
        # control socket itself remains group-restricted.
        RuntimeDirectoryMode = "0755";
        StateDirectory = "work-preview";
        StateDirectoryMode = "0750";
        ExecStart = lib.escapeShellArgs [
          "${cfg.package}/bin/work-preview"
          "serve"
          "--database"
          "/var/lib/work-preview/work-preview.db"
          "--domain"
          cfg.domain
          "--snippet-dir"
          cfg.snippetDirectory
          "--log-dir"
          cfg.logDirectory
          "--caddyfile"
          cfg.rootCaddyfile
          "--caddy-bin"
          "${pkgs.caddy}/bin/caddy"
          "--caddy-address"
          cfg.caddyAdminAddress
          "--ttl"
          cfg.ttl
          "--sweep-interval"
          cfg.sweepInterval
        ];
        Restart = "on-failure";
        RestartSec = 5;
        NoNewPrivileges = true;
        PrivateTmp = true;
        ProtectHome = true;
        ProtectSystem = "strict";
        ReadWritePaths = [cfg.logDirectory];
      };
    };
  };
}
