# GAPI nixos-generators Test Configuration

This directory contains configurations for testing GAPI using [nixos-generators](https://github.com/nix-community/nixos-generators) to create various bootable images.

## Quick Start

```bash
# Build ISO image with GAPI pre-installed
nix build .#iso

# Build VM image
nix build .#vm

# Build QEMU image
nix build .#qcow

# Build container image
nix build .#lxc
```

## Available Formats

- **iso** - Bootable ISO for USB/CD
- **vm** - QEMU VM with automatic boot
- **vm-nogui** - Headless QEMU VM
- **qcow** - QCOW2 disk image
- **raw** - Raw disk image
- **raw-efi** - Raw EFI disk image
- **virtualbox** - VirtualBox OVA
- **vmware** - VMware VMDK
- **lxc** - LXC container
- **docker** - Docker image
- **proxmox** - Proxmox VE template

## Testing Workflow

### 1. ISO Testing

```bash
# Build bootable ISO
nix build .#iso

# Boot in QEMU
qemu-system-x86_64 -cdrom ./result/iso/*.iso -m 4G

# Or write to USB
sudo dd if=./result/iso/*.iso of=/dev/sdX bs=4M status=progress
```

### 2. VM Testing

```bash
# Build and run VM
nix build .#vm
./result/bin/run-*-vm

# Inside VM, verify GAPI
systemctl status gapi
gapictl status
```

### 3. Container Testing

```bash
# Build LXC container
nix build .#lxc
nix build .#lxc-metadata

# Import into LXC
lxc image import ./result-metadata/tarball/*.tar.xz ./result/tarball/*.tar.xz --alias gapi-test
lxc launch gapi-test test-instance
lxc exec test-instance -- gapictl status
```

### 4. Cloud Image Testing

```bash
# Build for specific cloud provider
nix build .#amazon
nix build .#azure
nix build .#gce
nix build .#do  # Digital Ocean
```

## Configuration Files

- `base.nix` - Base GAPI configuration shared across all formats
- `iso.nix` - ISO-specific configuration (installer)
- `vm.nix` - VM-specific configuration
- `container.nix` - Container-specific configuration

## PID1 Testing

For PID1 testing, use the minimal configuration:

```bash
# Build minimal system with GAPI as primary supervisor
nix build .#pid1-test

# Run in QEMU
./result/bin/run-*-vm

# GAPI will be running as the main process supervisor
```

## Customization

Edit `base.nix` to customize the GAPI installation:

```nix
{
  services.gapi = {
    enable = true;
    agentsDir = "/var/lib/gapi/agents";
    logLevel = "debug";
  };
  
  # Add test agents
  environment.etc."gapi/agents/test.py.service".text = ''
    # ENABLED = True
    # TYPE = service
    def start():
        print("Test agent running")
  '';
}
```

## CI/CD Integration

```yaml
# .github/workflows/test-images.yml
name: Test Images
on: [push]
jobs:
  build-iso:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: cachix/install-nix-action@v22
      - run: nix build .#iso
      - run: nix build .#vm
```
