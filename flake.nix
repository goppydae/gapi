# Copyright (c) 2025 Steven Verhelle (enqack)
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.
#
# SPDX-License-Identifier: MPL-2.0

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
        # Formats every Linux host can produce.
        imageFormats = [
          "iso" "vm" "qcow" "raw" "docker" "lxc" "lxc-metadata"
          "vmware"
        ];

        # virtualbox's image builder pulls in the VirtualBox package,
        # which nixpkgs marks x86_64-linux only - so offering this format
        # on aarch64-linux advertises a target that cannot be built there.
        # Found by running 'nix flake check --all-systems', which nothing
        # in CI ran until GAPI-DIV-064.
        x86ImageFormats = [ "virtualbox" ];

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
          ]
          # DAEMON-SIDE ONLY, and the reason aarch64-darwin was declared
          # and unusable: criu and libseccomp have no darwin build, so
          # merely instantiating this shell there failed. The flake said
          # "aarch64-darwin stays for editing in the dev shell" while no
          # such shell could exist (GAPI-DIV-064).
          #
          # The split is the product's own: the control-plane client is
          # cross-platform - gapictl cross-compiles to darwin/arm64 today
          # - and the supervisor is not, because cgroups v2, PID 1 and
          # CRIU are the point rather than an implementation detail. A
          # darwin shell therefore builds both binaries, runs the unit
          # tests and lints; it cannot run the daemon meaningfully or the
          # privileged suites, neither of which it could have anyway.
          ++ lib.optionals stdenv.hostPlatform.isLinux [
            criu
            libseccomp
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

        # Linux-only, and honestly so: nix/package.nix declares
        # platforms.linux because the supervisor IS cgroups v2, PID 1 and
        # CRIU. Advertising a package on darwin that cannot be built there
        # makes 'nix flake check --all-systems' fail for a Mac developer
        # rather than simply find nothing to do - the same reasoning
        # nix/checks.nix already applies to the VM tests (GAPI-DIV-064).
        #
        # The dev shell IS offered on darwin, which is the whole point: a
        # Mac can build both binaries with the Go toolchain, run the unit
        # tests and lint. gapictl cross-compiles to darwin/arm64 today; a
        # packaged cross-platform client would be an additive output here,
        # not a change to this one.
        packages = pkgs.lib.optionalAttrs onLinux {
          default = pkgs.callPackage ./nix/package.nix { };
          gapi = self.packages.${system}.default;
        };

        # THE IMAGE FORMATS LIVE HERE AND NOT IN packages, AND THAT
        # PLACEMENT IS THE ENTIRE FIX FOR GAPI-DIV-068. `nix flake
        # check` names this output and then does not force its
        # attributes - it is the one well-known output nix deliberately
        # does not traverse, because nixpkgs itself is too large to
        # check. So the seventeen NixOS image fixpoints (nine formats on
        # x86_64-linux, eight on aarch64-linux) are no longer evaluated
        # by CI, which is what was exhausting the runner: measured at a
        # 5.2 GiB peak and OOM-killed under a 4 GiB cap, against 0.6 GiB
        # without them.
        #
        # NOTHING ELSE CHANGES FOR AN OPERATOR. `nix build .#iso` still
        # resolves, because the installable attribute paths nix tries
        # are packages.<system>.<name> AND legacyPackages.<system>.<name>
        # in that order; docs/installation.md and
        # nix/generators/README.md keep their commands verbatim. The one
        # visible difference is `nix flake show`, which prints
        # "omitted (use '--legacy' to show)" for this output.
        #
        # The cost is real and is recorded rather than hidden: NO CI JOB
        # EVALUATES THE IMAGE FORMATS AT ALL any more, so a format that
        # stops evaluating is found by whoever next builds one locally.
        # What CI keeps is checks.<system>.image-credentials below,
        # which evaluates the same base configuration ONCE - that is the
        # reader GAPI-DIV-069 needs, and it deliberately survives this
        # move.
        legacyPackages = pkgs.lib.optionalAttrs onLinux (
          # nix build .#iso, .#vm, .#qcow, ... - the targets
          # nix/generators/README.md has documented all along and the
          # flake never exposed.
          pkgs.lib.genAttrs imageFormats mkImage
        ) // pkgs.lib.optionalAttrs (onLinux && pkgs.stdenv.hostPlatform.isx86_64) (
          pkgs.lib.genAttrs x86ImageFormats mkImage
        );

        # VM-backed checks. Booting a guest is the only way to assert the
        # module's runtime behaviour rather than merely evaluating it.
        checks = import ./nix/checks.nix { inherit pkgs self nixpkgs; };
      }
    );
}
