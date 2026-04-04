{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
  };

  outputs =
    { nixpkgs, ... }:
    let
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
            version = "0.1.0";
            src = ./.;
            vendorHash = "sha256-fH93/19rZY/jduF4ZS0RLrqBWdCjz6XYnoN+3KPd4Lg=";
            nativeCheckInputs = [ pkgs.git ];
          };
        }
      );
    };
}
