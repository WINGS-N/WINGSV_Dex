{
  description = "Local Nix flake for WINGSV_Dex";

  inputs = {
    # Local store path avoids network during local experiments in this environment.
    # Change back to github:NixOS/nixpkgs/nixos-unstable before upstreaming if desired.
    nixpkgs.url = "path:/nix/store/kfs4nfy1705ksbn0gv15ls921cc90h2z-source";
  };

  outputs = { self, nixpkgs }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs { inherit system; };

      nativeLibs = with pkgs; [ gtk4 webkitgtk_6_0 libsoup_3 ];

      wails3 = pkgs.buildGoModule rec {
        pname = "wails3";
        version = "3.0.0-alpha2.113";

        src = pkgs.fetchFromGitHub {
          owner = "wailsapp";
          repo = "wails";
          rev = "v${version}";
          hash = "sha256-REqUhSTlFwCcosxDNUhZegwIKWVuNbYShFoxHfLUYpY=";
        };

        nativeBuildInputs = [ pkgs.pkg-config ];
        buildInputs = nativeLibs;

        env.GOWORK = "off";
        modRoot = "v3";
        subPackages = [ "cmd/wails3" ];
        proxyVendor = true;
        vendorHash = "sha256-6c/UAe3nKBDKU160IzDdYkjEWZo2pdH0w+pYTjjdyF0=";
        doCheck = false;
      };
    in {
      packages.${system}.wails3 = wails3;

      devShells.${system}.default = pkgs.mkShell {
        packages = with pkgs; [
          go_1_25
          nodejs
          pnpm
          go-task
          pkg-config
          gcc
          git
          nftables
          protobuf
          protoc-gen-go
          protoc-gen-go-grpc
        ] ++ nativeLibs ++ [
          wails3
        ];

        shellHook = ''
          export CGO_ENABLED=1
          export GOFLAGS="-buildvcs=false"
          echo "WINGSV_Dex Nix dev shell"
          echo "Try: task build:vkturn && task build && task run"
        '';
      };
    };
}
