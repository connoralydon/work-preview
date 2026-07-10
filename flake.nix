{
  description = "Ephemeral Caddy previews for same-host development servers";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = {
    self,
    nixpkgs,
  }: let
    systems = ["x86_64-linux" "aarch64-linux"];
    forAllSystems = nixpkgs.lib.genAttrs systems;
  in {
    packages = forAllSystems (system: let
      pkgs = nixpkgs.legacyPackages.${system};
    in {
      default = pkgs.buildGoModule {
        pname = "work-preview";
        version = "0.1.0";
        src = self;
        vendorHash = "sha256-NDOtGXPKkjSgO8n+r66qeGWkqx4gTEBAcYRz33qAqH4=";
        subPackages = ["cmd/work-preview"];
        meta.mainProgram = "work-preview";
      };
    });

    apps = forAllSystems (system: {
      default = {
        type = "app";
        program = "${self.packages.${system}.default}/bin/work-preview";
      };
    });

    checks = forAllSystems (system: let
      pkgs = nixpkgs.legacyPackages.${system};
      moduleConfig =
        (nixpkgs.lib.nixosSystem {
          inherit system;
          modules = [
            self.nixosModules.default
            {
              system.stateVersion = "26.05";
              services.work-preview = {
                enable = true;
                package = pkgs.writeShellScriptBin "work-preview" "exit 0";
                tlsCertificateFile = "/run/secrets/cloudflare.crt";
                tlsCertificateKeyFile = "/run/secrets/cloudflare.key";
                groupMembers = ["agent"];
              };
            }
          ];
        }).config.systemd.services.work-preview.serviceConfig.ExecStart;
    in {
      inherit (self.packages.${system}) default;
      module = pkgs.runCommand "work-preview-module-check" {
        service = moduleConfig;
      } "touch $out";
    });

    devShells = forAllSystems (system: let
      pkgs = nixpkgs.legacyPackages.${system};
    in {
      default = pkgs.mkShell {
        packages = with pkgs; [
          caddy
          go
          gopls
          gotools
          mariadb
          protobuf
          protoc-gen-go
          protoc-gen-go-grpc
        ];
      };
    });

    formatter = forAllSystems (system: nixpkgs.legacyPackages.${system}.alejandra);

    nixosModules.default = import ./nix/module.nix {inherit self;};
  };
}
