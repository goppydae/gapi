{
  description = "GAPI (GoPPydae Agent Programming Interface) - Agent supervision framework with event-driven daemon management";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
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
              mdformat
              mdformat-gfm
              mdformat-frontmatter
              mdformat-footnote
              jsonschema
              pybindgen
            ]))
            
            # Markdown linting
            markdownlint-cli2
          ];

          shellHook = ''
            export GOBIN=$PWD/.bin
            export PATH=$GOBIN:$PATH

            if [ ! -x "$GOBIN/gopy" ]; then
              echo "Building pinned gopy from tools/gopy..."
              (cd tools/gopy && GOWORK=off go build -o "$GOBIN/gopy" github.com/go-python/gopy)
            fi

            echo "🏗️  GAPI (GoPPydae Agent Programming Interface) - Agent Supervision Framework"
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
      }
    );
}
