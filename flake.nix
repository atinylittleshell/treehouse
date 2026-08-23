{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
  };

  outputs =
    { nixpkgs, ... }:
    let
      version = "2.3.0"; # x-release-please-version
      systems = [
        "aarch64-darwin"
        "x86_64-darwin"
        "aarch64-linux"
        "x86_64-linux"
      ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in
    {
      packages = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = pkgs.buildGoModule {
            pname = "treehouse";
            inherit version;
            src = ./.;
            vendorHash = "sha256-z8IndcHcZ6nLqhLtAYul3ppddpOA4AHGQWIlfYY/pfI=";
            ldflags = [
              "-X main.version=v${version}"
            ];
            # git for the VCS tests; python3 for the no-mistakes gate
            # script's attestation parser, which the gate tests execute
            # (GitHub runners have python3 ambiently, the nix sandbox does
            # not - see issue #115).
            nativeCheckInputs = [ pkgs.git pkgs.python3 ];
          };
        }
      );
    };
}
