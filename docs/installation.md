# GAPI Installation Guide

This guide covers multiple installation methods for GAPI, including NixOS deployment, systemd integration, and PID1 testing for container environments.

## Table of Contents

- [Quick Start](#quick-start)
- [NixOS Installation](#nixos-installation)
- [Systemd Service](#systemd-service)
- [Container Deployment](#container-deployment)
- [PID1 Testing on NixOS](#pid1-testing-on-nixos)
- [Development Installation](#development-installation)

---

## Quick Start

### From Source (Nix)

```bash
# Clone the repository
git clone https://github.com/goppydae/gapi.git
cd gapi

# Build with Nix
nix develop -c mage build

# Install binaries
sudo install -m 755 bin/gapid /usr/local/bin/
sudo install -m 755 bin/gapictl /usr/local/bin/
```

### From Source (Go)

```bash
# Prerequisites: Go 1.23+, GCC, Python 3
go build -o bin/gapid ./cmd/gapid
go build -o bin/gapictl ./cmd/gapictl

sudo install -m 755 bin/gapid /usr/local/bin/
sudo install -m 755 bin/gapictl /usr/local/bin/
```

---

## NixOS Installation

### Method 1: NixOS Module (Recommended)

Create a NixOS module for GAPI:

**`/etc/nixos/modules/gapi.nix`:**

```nix
{ config, lib, pkgs, ... }:

with lib;

let
  cfg = config.services.gapi;
  
  # Build GAPI from source
  gapi = pkgs.buildGoModule rec {
    pname = "gapi";
    version = "0.1.0";
    
    src = /path/to/gapi/source;  # Update this path
    
    vendorHash = null;  # Update after first build
    
    buildInputs = with pkgs; [ gcc python3 ];
    
    buildPhase = ''
      export HOME=$TMPDIR
      go build -o gapid ./cmd/gapid
      go build -o gapictl ./cmd/gapictl
    '';
    
    installPhase = ''
      mkdir -p $out/bin
      install -m 755 gapid $out/bin/
      install -m 755 gapictl $out/bin/
    '';
  };

in {
  options.services.gapi = {
    enable = mkEnableOption "GAPI agent supervision framework";
    
    package = mkOption {
      type = types.package;
      default = gapi;
      description = "GAPI package to use";
    };
    
    agentsDir = mkOption {
      type = types.path;
      default = "/var/lib/gapi/agents";
      description = "Directory containing agent definitions";
    };
    
    configFile = mkOption {
      type = types.nullOr types.path;
      default = null;
      description = "Path to config.yaml";
    };
    
    user = mkOption {
      type = types.str;
      default = "gapi";
      description = "User to run GAPI as";
    };
    
    group = mkOption {
      type = types.str;
      default = "gapi";
      description = "Group to run GAPI as";
    };
  };

  config = mkIf cfg.enable {
    users.users.${cfg.user} = {
      isSystemUser = true;
      group = cfg.group;
      description = "GAPI service user";
      home = "/var/lib/gapi";
      createHome = true;
    };
    
    users.groups.${cfg.group} = {};
    
    systemd.services.gapi = {
      description = "GAPI Agent Supervision Framework";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];
      
      serviceConfig = {
        Type = "simple";
        User = cfg.user;
        Group = cfg.group;
        ExecStart = "${cfg.package}/bin/gapid ${optionalString (cfg.configFile != null) "-config ${cfg.configFile}"}";
        Restart = "on-failure";
        RestartSec = "5s";
        
        # Security hardening
        NoNewPrivileges = true;
        PrivateTmp = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        ReadWritePaths = [ "/var/lib/gapi" ];
        
        # Resource limits
        LimitNOFILE = 65536;
        LimitNPROC = 512;
      };
      
      environment = {
        GAPI_AGENTS_DIR = cfg.agentsDir;
      };
    };
    
    # Create agents directory
    systemd.tmpfiles.rules = [
      "d /var/lib/gapi 0750 ${cfg.user} ${cfg.group} -"
      "d ${cfg.agentsDir} 0750 ${cfg.user} ${cfg.group} -"
    ];
  };
}
```

**Enable in `/etc/nixos/configuration.nix`:**

```nix
{ config, pkgs, ... }:

{
  imports = [
    ./modules/gapi.nix
  ];
  
  services.gapi = {
    enable = true;
    agentsDir = "/var/lib/gapi/agents";
  };
}
```

**Rebuild and activate:**

```bash
sudo nixos-rebuild switch
sudo systemctl status gapi
```

### Method 2: Flake-based Installation

Add to your `flake.nix`:

```nix
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    gapi.url = "github:goppydae/gapi";  # Update when published
  };
  
  outputs = { self, nixpkgs, gapi }: {
    nixosConfigurations.myhost = nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      modules = [
        gapi.nixosModules.default
        {
          services.gapi.enable = true;
        }
      ];
    };
  };
}
```

---

## Systemd Service

For non-NixOS systems, create a systemd service manually:

**`/etc/systemd/system/gapi.service`:**

```ini
[Unit]
Description=GAPI Agent Supervision Framework
Documentation=https://github.com/goppydae/gapi
After=network.target

[Service]
Type=simple
User=gapi
Group=gapi
ExecStart=/usr/local/bin/gapid
Restart=on-failure
RestartSec=5s

# Environment
Environment="GAPI_AGENTS_DIR=/var/lib/gapi/agents"

# Working directory
WorkingDirectory=/var/lib/gapi

# Security
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/gapi

# Resource limits
LimitNOFILE=65536
LimitNPROC=512

[Install]
WantedBy=multi-user.target
```

**Setup and enable:**

```bash
# Create user and directories
sudo useradd -r -s /bin/false -d /var/lib/gapi gapi
sudo mkdir -p /var/lib/gapi/agents
sudo chown -R gapi:gapi /var/lib/gapi

# Enable and start service
sudo systemctl daemon-reload
sudo systemctl enable --now gapi.service
sudo systemctl status gapi
```

---

## Container Deployment

### Docker

**`Dockerfile`:**

```dockerfile
FROM nixos/nix:latest AS builder

WORKDIR /build
COPY . .

RUN nix develop -c mage build

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y \
    ca-certificates \
    python3 \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /build/bin/gapid /usr/local/bin/
COPY --from=builder /build/bin/gapictl /usr/local/bin/

RUN useradd -r -s /bin/false gapi && \
    mkdir -p /var/lib/gapi/agents && \
    chown -R gapi:gapi /var/lib/gapi

USER gapi
WORKDIR /var/lib/gapi

EXPOSE 4242

CMD ["/usr/local/bin/gapid"]
```

**Build and run:**

```bash
docker build -t gapi:latest .
docker run -d \
  --name gapi \
  -v /path/to/agents:/var/lib/gapi/agents \
  -p 4242:4242 \
  gapi:latest
```

### NixOS Container

```nix
containers.gapi = {
  autoStart = true;
  privateNetwork = false;
  
  config = { config, pkgs, ... }: {
    services.gapi = {
      enable = true;
      agentsDir = "/var/lib/gapi/agents";
    };
  };
};
```

---

## nixos-generators - Multi-Format Image Builder

[nixos-generators](https://github.com/nix-community/nixos-generators) allows creating bootable images in multiple formats from a single NixOS configuration. This is perfect for testing GAPI across different deployment scenarios.

### Quick Start

The GAPI flake includes pre-configured nixos-generators outputs:

```bash
# Build bootable ISO
nix build .#iso

# Build QEMU VM
nix build .#vm
./result/bin/run-*-vm

# Build QCOW2 image
nix build .#qcow

# Build Docker container
nix build .#docker

# Build LXC container
nix build .#lxc
```

### Available Formats

| Format | Command | Use Case |
|--------|---------|----------|
| **iso** | `nix build .#iso` | Bootable USB/CD installer |
| **vm** | `nix build .#vm` | QEMU VM with GUI |
| **vm-nogui** | `nix build .#vm-nogui` | Headless QEMU VM |
| **qcow** | `nix build .#qcow` | QCOW2 for QEMU/KVM |
| **raw** | `nix build .#raw` | Raw disk image |
| **raw-efi** | `nix build .#raw-efi` | EFI-bootable raw image |
| **virtualbox** | `nix build .#virtualbox` | VirtualBox OVA |
| **vmware** | `nix build .#vmware` | VMware VMDK |
| **lxc** | `nix build .#lxc` | LXC container |
| **docker** | `nix build .#docker` | Docker image |

### Testing Workflow

#### 1. ISO Testing

```bash
# Build ISO
nix build .#iso

# Test in QEMU
qemu-system-x86_64 \
  -cdrom ./result/iso/*.iso \
  -m 4G \
  -enable-kvm

# Or write to USB
sudo dd if=./result/iso/*.iso of=/dev/sdX bs=4M status=progress
```

#### 2. VM Testing

```bash
# Build and run
nix build .#vm
./result/bin/run-*-vm

# Inside VM
systemctl status gapi
gapictl status

# Test agents are pre-configured:
# - heartbeat.py.timer (runs every 30s)
# - sysinfo.py.service (long-running service)
```

#### 3. Container Testing (LXC)

```bash
# Build container and metadata
nix build .#lxc
nix build .#lxc-metadata

# Import into LXC
lxc image import \
  ./result-metadata/tarball/*.tar.xz \
  ./result/tarball/*.tar.xz \
  --alias gapi-test

# Launch and test
lxc launch gapi-test test-instance
lxc exec test-instance -- systemctl status gapi
lxc exec test-instance -- gapictl status
```

#### 4. Docker Testing

```bash
# Build Docker image
nix build .#docker

# Load and run
docker load < ./result
docker run -it nixos:latest

# Inside container
systemctl status gapi
gapictl status
```

### Customization

Edit `nix/generators/base.nix` to customize the configuration:

```nix
{
  services.gapi = {
    enable = true;
    logLevel = "debug";  # Change log level
  };
  
  # Add custom agents
  environment.etc."gapi/agents/myagent.py.service".text = ''
    # ENABLED = True
    # TYPE = service
    def start():
        print("Custom agent")
  '';
}
```

### CI/CD Integration

```yaml
# .github/workflows/test-images.yml
name: Test Images
on: [push, pull_request]

jobs:
  build-images:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        format: [iso, vm, qcow, docker]
    steps:
      - uses: actions/checkout@v3
      - uses: cachix/install-nix-action@v22
        with:
          extra_nix_config: |
            experimental-features = nix-command flakes
      - name: Build ${{ matrix.format }}
        run: nix build .#${{ matrix.format }}
```

### Advanced: Custom Formats

Create custom image formats by adding to `flake.nix`:

```nix
packages.custom = nixos-generators.nixosGenerate {
  inherit system;
  modules = [
    ./nix/generators/base.nix
    {
      # Custom configuration
      services.gapi.logLevel = "debug";
      virtualisation.diskSize = 20 * 1024;  # 20GB
    }
  ];
  format = "qcow";
};
```

---

## PID1 Testing on NixOS


Testing GAPI as PID1 is useful for understanding its behavior as a system supervisor, similar to systemd or runit.

### Method 1: NixOS VM with Custom Init

Create a minimal NixOS configuration that uses GAPI as PID1:

**`gapi-pid1-test.nix`:**

```nix
{ config, pkgs, lib, ... }:

let
  gapi = pkgs.buildGoModule {
    pname = "gapi";
    version = "0.1.0";
    src = /path/to/gapi/source;
    vendorHash = null;
    buildInputs = [ pkgs.gcc pkgs.python3 ];
    buildPhase = ''
      go build -o gapid ./cmd/gapid
      go build -o gapictl ./cmd/gapictl
    '';
    installPhase = ''
      mkdir -p $out/bin
      install -m 755 gapid $out/bin/
      install -m 755 gapictl $out/bin/
    '';
  };

in {
  # Use GAPI as PID1
  boot.isContainer = true;
  boot.initrd.enable = false;
  
  # Override init to use gapid
  system.build.toplevel = lib.mkForce (
    pkgs.runCommand "gapi-pid1-system" {} ''
      mkdir -p $out/bin
      ln -s ${gapi}/bin/gapid $out/init
    ''
  );
  
  # Minimal environment
  environment.systemPackages = [ gapi pkgs.coreutils ];
  
  # Disable systemd
  systemd.package = lib.mkForce pkgs.emptyDirectory;
}
```

**Build and run VM:**

```bash
# Build the configuration
nixos-rebuild build-vm -I nixos-config=./gapi-pid1-test.nix

# Run the VM
./result/bin/run-nixos-vm
```

### Method 2: systemd-nspawn Container

```bash
# Create a minimal NixOS container
sudo nixos-container create gapi-test --config-file /etc/nixos/containers/gapi-test.nix

# Start container
sudo nixos-container start gapi-test

# Enter container
sudo nixos-container root-login gapi-test

# Inside container, replace init
sudo systemctl stop gapi
sudo ln -sf /usr/local/bin/gapid /sbin/init
sudo reboot
```

### Method 3: QEMU with Direct Kernel Boot

```bash
# Build a minimal initrd with gapid
nix-build -E '
  with import <nixpkgs> {};
  makeInitrd {
    contents = [
      { object = gapi;
        symlink = "/init";
      }
    ];
  }
'

# Boot with QEMU
qemu-system-x86_64 \
  -kernel /path/to/kernel \
  -initrd ./result \
  -append "init=/init console=ttyS0" \
  -nographic
```

### Method 4: NixOS Test Framework

Create a NixOS test for PID1 behavior:

**`test-gapi-pid1.nix`:**

```nix
import <nixpkgs/nixos/tests/make-test-python.nix> ({ pkgs, ... }: {
  name = "gapi-pid1-test";
  
  nodes.machine = { config, pkgs, ... }: {
    services.gapi.enable = true;
    
    # Add test agents
    environment.etc."gapi/agents/test.py.service".text = ''
      # ENABLED = True
      # TYPE = service
      
      def start():
          print("Test agent running")
          import time
          while True:
              time.sleep(60)
    '';
  };
  
  testScript = ''
    machine.wait_for_unit("gapi.service")
    machine.succeed("gapictl status")
    machine.succeed("gapictl status | grep -q test")
  '';
})
```

**Run the test:**

```bash
nix-build test-gapi-pid1.nix
./result/bin/nixos-test-driver
```

---

## Development Installation

For development and testing:

```bash
# Clone repository
git clone https://github.com/goppydae/gapi.git
cd gapi

# Enter Nix development shell
nix develop

# Build
mage build

# Run locally (foreground)
./bin/gapid

# In another terminal
./bin/gapictl status
```

### Hot Reload with Air

```bash
# Install air
go install github.com/cosmtrek/air@latest

# Run with hot reload
air
```

---

## Configuration

After installation, create `/var/lib/gapi/config.yaml`:

```yaml
transport:
  type: quic
  address: 127.0.0.1:4242
  certFile: /var/lib/gapi/certs/server.crt
  keyFile: /var/lib/gapi/certs/server.key

security:
  verifyKey: /var/lib/gapi/keys/verify.pub  # Optional

logging:
  level: info
  format: json
```

Generate certificates:

```bash
# Self-signed for testing
openssl req -x509 -newkey rsa:4096 \
  -keyout /var/lib/gapi/certs/server.key \
  -out /var/lib/gapi/certs/server.crt \
  -days 365 -nodes \
  -subj "/CN=localhost"
```

---

## Verification

Test your installation:

```bash
# Check service status
sudo systemctl status gapi

# List agents
gapictl status

# View logs
sudo journalctl -u gapi -f

# Test agent deployment
cat > /var/lib/gapi/agents/test.py.service <<EOF
# ENABLED = True
# TYPE = service

def start():
    print("Hello from GAPI!")
    import time
    while True:
        time.sleep(60)
EOF

# Verify agent is discovered
gapictl status | grep test
```

---

## Troubleshooting

### Service won't start

```bash
# Check logs
sudo journalctl -u gapi -n 50

# Verify binary
which gapid
gapid --version

# Check permissions
ls -la /var/lib/gapi
```

### Agents not discovered

```bash
# Verify agents directory
ls -la /var/lib/gapi/agents

# Check agent syntax
gapictl validate /var/lib/gapi/agents/myagent.py.service
```

### PID1 testing issues

- Ensure kernel supports necessary features (cgroups v2, namespaces)
- Check for missing dependencies in minimal environments
- Verify GAPI handles signals correctly (SIGTERM, SIGCHLD)

---

## Next Steps

- [Agent Development Guide](./AGENTS.md)
- [Security Best Practices](./SECURITY.md)
- [Production Deployment](./DEPLOYMENT.md)
