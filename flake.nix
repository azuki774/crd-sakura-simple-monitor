{
  description = "Development environment for crd-sakura-simple-monitor";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.05";
  };

  outputs = { self, nixpkgs }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs {
        inherit system;
      };
      staticPkgs = pkgs.pkgsStatic;
    in {
      devShells.${system}.default = pkgs.mkShell {
        packages = with pkgs; [
          staticPkgs.go_1_24
          kubebuilder
          kubectl
          git
          gnumake
        ];

        shellHook = ''
          echo "Loaded dev shell for crd-sakura-simple-monitor"
          go version
          kubebuilder version
          kubectl version --client
        '';
      };
    };
}
