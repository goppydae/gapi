{
  description = "GAPI - Agent supervision framework with event-driven daemon management";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    nixos-generators = {
      url = "github:nix-community/nixos-generators";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { self, nixpkgs, flake-utils, nixos-generators }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs {
          inherit system;
          config.allowUnfree = true;
        };
        
        gapi = pkgs.callPackage ./nix/package.nix {};
        
      in {
        # Package output
        packages = {
          default = gapi;
          gapi = gapi;
          
          # nixos-generators images for testing
          iso = nixos-generators.nixosGenerate {
            inherit system;
            modules = [ ./nix/generators/base.nix ];
            format = "iso";
          };
          
          vm = nixos-generators.nixosGenerate {
            inherit system;
            modules = [ ./nix/generators/base.nix ];
            format = "vm";
          };
          
          vm-nogui = nixos-generators.nixosGenerate {
            inherit system;
            modules = [ ./nix/generators/base.nix ];
            format = "vm-nogui";
          };
          
          qcow = nixos-generators.nixosGenerate {
            inherit system;
            modules = [ 
              ./nix/generators/base.nix
              { virtualisation.diskSize = 10 * 1024; }  # 10GB
            ];
            format = "qcow";
          };
          
          raw = nixos-generators.nixosGenerate {
            inherit system;
            modules = [ ./nix/generators/base.nix ];
            format = "raw";
          };
          
          raw-efi = nixos-generators.nixosGenerate {
            inherit system;
            modules = [ ./nix/generators/base.nix ];
            format = "raw-efi";
          };
          
          virtualbox = nixos-generators.nixosGenerate {
            inherit system;
            modules = [ ./nix/generators/base.nix ];
            format = "virtualbox";
          };
          
          vmware = nixos-generators.nixosGenerate {
            inherit system;
            modules = [ ./nix/generators/base.nix ];
            format = "vmware";
          };
          
          lxc = nixos-generators.nixosGenerate {
            inherit system;
            modules = [ ./nix/generators/base.nix ];
            format = "lxc";
          };
          
          lxc-metadata = nixos-generators.nixosGenerate {
            inherit system;
            modules = [ ./nix/generators/base.nix ];
            format = "lxc-metadata";
          };
          
          docker = nixos-generators.nixosGenerate {
            inherit system;
            modules = [ ./nix/generators/base.nix ];
            format = "docker";
          };
        };
        
        # Development shell
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gcc
            mage
            openssl
            protobuf
            protoc-gen-go
            protoc-gen-go-grpc
            pam
            pkg-config
            python3
            python3Packages.pybindgen
          ];

          shellHook = ''
            # Explicitly add build inputs to PATH
            export PATH=${pkgs.gcc}/bin:${pkgs.go}/bin:${pkgs.python3}/bin:$PATH

            export GOBIN=$PWD/.bin
            export PATH=$GOBIN:$PATH

            # Uncomment if needed for QUIC
            # sudo sysctl -w net.core.rmem_max=7500000
            # sudo sysctl -w net.core.wmem_max=7500000

            if ! command -v gopy &> /dev/null; then
              echo "Installing gopy..."
              go install github.com/go-python/gopy@latest
            fi

            if [ -n "$ZSH_VERSION" ]; then
              PROMPT="$PROMPT (nix-shell)"
            else
              export PS1="$PS1 (nix-shell)"
            fi
            echo "Welcome to the GoPPydae dev shell. Goblin stands ready."
            echo ""
            echo "Available nixos-generators formats:"
            echo "  nix build .#iso          - Bootable ISO"
            echo "  nix build .#vm           - QEMU VM"
            echo "  nix build .#qcow         - QCOW2 image"
            echo "  nix build .#docker       - Docker image"
            echo "  nix build .#lxc          - LXC container"
          '';
        };
        
        # Apps for easy running
        apps = {
          default = {
            type = "app";
            program = "${gapi}/bin/gapid";
          };
          gapid = {
            type = "app";
            program = "${gapi}/bin/gapid";
          };
          gapictl = {
            type = "app";
            program = "${gapi}/bin/gapictl";
          };
        };
      }
    ) // {
      # NixOS module (system-independent)
      nixosModules.default = import ./nix/module.nix;
      nixosModules.gapi = import ./nix/module.nix;
    };
}
