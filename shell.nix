{ pkgs ? import <nixpkgs> { config.allowUnfree = true; } }:

pkgs.mkShell {
  buildInputs = with pkgs; [
    go
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
}
