{
  description = "Personal local OAuth callback dispatcher for dynamic development environments";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
        pname = "oauth-callback-dispatcher";
        version = "0.1.0";
      in
      {
        packages.default = pkgs.buildGoModule {
          inherit pname version;
          src = ./.;
          vendorHash = null;

          ldflags = [
            "-s"
            "-w"
          ];

          meta = {
            description = "Local development OAuth callback dispatcher";
            license = pkgs.lib.licenses.mit;
            mainProgram = pname;
          };
        };

        apps.default = {
          type = "app";
          program = "${self.packages.${system}.default}/bin/${pname}";
        };

        devShells.default = pkgs.mkShell {
          packages = [
            pkgs.go_1_25
            pkgs.golangci-lint
            pkgs.goreleaser
          ];
        };
      });
}
