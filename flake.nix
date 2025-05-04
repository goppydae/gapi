{
  description = "GAPI dev shell with Go 1.23, protobuf tools, and build tools";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs {
          inherit system;
          config.allowUnfree = true;
        };
      in {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go_1_23
            air
            mage
            buf
            openssl
            protoc-gen-go
            protoc-gen-go-grpc
            pam
          ];

          shellHook = ''
            export GOBIN=$PWD/.bin
            export PATH=$GOBIN:$PATH

            sudo sysctl -w net.core.rmem_max=7500000
            sudo sysctl -w net.core.wmem_max=7500000

            if [ -n "$ZSH_VERSION" ]; then
              PROMPT="$PROMPT (nix-shell)"
            else
              export PS1="$PS1 (nix-shell)"
            fi
            echo "Welcome to the GoPPydae dev shell. Goblin stands ready."
          '';
        };
      });
}
