{
  description = "GAPI (GoPPydae Agent Process Infrastructure) - Agent supervision framework with event-driven daemon management";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    nixos-generators = {
      url = "github:nix-community/nixos-generators";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { self, nixpkgs, flake-utils, nixos-generators }:
    # System-independent outputs live OUTSIDE eachDefaultSystem. A NixOS
    # module is evaluated by the consuming system's module system, so it
    # must not be keyed by ours: nesting it here would produce
    # nixosModules.<system>.default, which no consumer can import.
    {
      nixosModules.default = import ./nix/module.nix;
      nixosModules.gapi = self.nixosModules.default;
    }
    //
    # Explicit system list rather than eachDefaultSystem. nixpkgs 26.11
    # dropped x86_64-darwin, so enumerating the default set now throws
    # while merely instantiating pkgs for it. This is a Linux product
    # anyway - PID 1, cgroups v2, CRIU - and nix/package.nix already
    # declares platforms.linux; aarch64-darwin stays for editing in the
    # dev shell.
    flake-utils.lib.eachSystem [ "x86_64-linux" "aarch64-linux" "aarch64-darwin" ] (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};

        # nixos-generators and NixOS VM tests are Linux-only. Advertising
        # them on darwin would make 'nix flake check' fail for anyone on a
        # Mac rather than simply find nothing to do.
        onLinux = pkgs.stdenv.hostPlatform.isLinux;

        # The image formats nix/generators/README.md already documents.
        imageFormats = [
          "iso" "vm" "qcow" "raw" "docker" "lxc" "lxc-metadata"
          "virtualbox" "vmware"
        ];

        mkImage = format: nixos-generators.nixosGenerate {
          inherit system format;
          modules = [ ./nix/generators/base.nix ];
        };
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            # Go toolchain
            go
            gotools # for goimports
            
            # CGO and build essentials
            gcc
            pkg-config
            pam
            openssl
            
            # Protocol Buffers
            protobuf
            protoc-gen-go
            protoc-gen-go-grpc
            buf

            # Checkpoint/restore. core/checkpoint resolves the criu
            # binary with exec.LookPath and returns ErrNoCriu without it,
            # so the dev shell could not exercise any checkpoint path -
            # only the error branch. libseccomp is criu's own pkg-config
            # dependency. Kept in step with goblin's shell, which
            # 'mage envcheck' compares against.
            criu
            libseccomp

            # Lint and security gate
            golangci-lint
            gosec
            govulncheck

            # Documentation toolchain
            mkdocs
            pandoc

            # Container orchestration
            podman
            podman-compose
            
            # Utilities
            rsync
            mage
            
            # Python verification tools
            (python3.withPackages (ps: with ps; [
              pytest
              jsonschema
              pybindgen
            ]))
            
            # Markdown linting
            markdownlint-cli2
          ];

          shellHook = ''
            # goppydae modules are private: skip proxy/sumdb, fetch direct.
            export GOPRIVATE=github.com/goppydae
            export GOBIN=$PWD/.bin
            export PATH=$GOBIN:$PATH

            # gcc 15 defaults to C23, where 'bool' is a keyword; the pinned
            # gopy's generated cgo preamble (typedef uint8_t bool) assumes
            # C17. Pin the dialect until gopy emits C23-safe code.
            export CGO_CFLAGS=-std=gnu17

            if [ ! -x "$GOBIN/gopy" ]; then
              echo "Building pinned gopy from tools/gopy..."
              (cd tools/gopy && GOWORK=off go build -o "$GOBIN/gopy" github.com/go-python/gopy)
            fi

            echo "GAPI (GoPPydae Agent Process Infrastructure) - Agent Supervision Framework"
            echo ""
            echo "Available mage tasks:"
            echo "  mage build       - Build gapid and gapictl binaries"
            echo "  mage test        - Run all tests"
            echo "  mage testUnit    - Run unit tests only"
            echo "  mage testADK     - Run ADK integration tests"
            echo "  mage testE2E     - Run end-to-end tests"
            echo ""
            echo "Run 'mage -l' to see all available tasks"
          '';
        };

        packages = {
          default = pkgs.callPackage ./nix/package.nix { };
          gapi = self.packages.${system}.default;
        } // pkgs.lib.optionalAttrs onLinux (
          # nix build .#iso, .#vm, .#qcow, ... - the targets
          # nix/generators/README.md has documented all along and the
          # flake never exposed.
          pkgs.lib.genAttrs imageFormats mkImage
        );

        # VM-backed checks. Booting a guest is the only way to assert the
        # module's runtime behaviour rather than merely evaluating it.
        checks = import ./nix/checks.nix { inherit pkgs self; };
      }
    );
}
