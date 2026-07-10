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
        vendorHash = "sha256-jSLflxFT1q5XF3qQj4hu+BGJWuaxz4ddG3D3Vl2v3H4=";
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
      evaluatedModule =
        (nixpkgs.lib.nixosSystem {
          inherit system;
          modules = [
            self.nixosModules.default
            {
              system.stateVersion = "26.05";
              services.work-preview = {
                enable = true;
                package = pkgs.writeShellScriptBin "work-preview" "exit 0";
                groupMembers = ["agent"];
              };
            }
          ];
        }).config;
      moduleCommand = evaluatedModule.systemd.services.work-preview.serviceConfig.ExecStart;
    in {
      inherit (self.packages.${system}) default;
      module = pkgs.runCommand "work-preview-module-check" {
        service = moduleCommand;
        embeddedDatabase =
          if
            nixpkgs.lib.hasInfix "--database /var/lib/work-preview/work-preview.db" moduleCommand
            && !nixpkgs.lib.hasInfix "mysql" moduleCommand
          then "yes"
          else throw "work-preview must use its embedded SQLite database";
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
          protobuf
          protoc-gen-go
          protoc-gen-go-grpc
          sqlite
        ];
      };
    });

    formatter = forAllSystems (system: nixpkgs.legacyPackages.${system}.alejandra);

    nixosModules.default = import ./nix/module.nix {inherit self;};
  };
}
