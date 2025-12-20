# Legacy shell.nix for non-flake users
# For flake users, use: nix develop
{ pkgs ? import <nixpkgs> { config.allowUnfree = true; } }:

pkgs.mkShell {
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

    if ! command -v gopy &> /dev/null; then
      echo "Installing gopy..."
      go install github.com/go-python/gopy@latest
    fi

    if [ -n "$ZSH_VERSION" ]; then
      PROMPT="$PROMPT (nix-shell)"
    else
      export PS1="$PS1 (nix-shell)"
    fi
    echo "Welcome to the GoPPydae dev shell. GAPI stands ready."
  '';
}
