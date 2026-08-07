# GAPI nixos-generators Test Configuration

This directory contains configurations for testing GAPI using [nixos-generators](https://github.com/nix-community/nixos-generators) to create various bootable images.

## There is no default login

`base.nix` provisions no credential: no password, no autologin, and SSH
set to `PermitRootLogin = "prohibit-password"` with
`PasswordAuthentication = false`. Every image below boots and runs the
service, and none of them will let you in until you add an authorized
key to your own module or provision through cloud-init.

Earlier images set `users.users.root.password = "gapi"` in the clear
(GAPI-DIV-069). `nix flake check` now runs
`checks.<system>.image-credentials`, which evaluates this configuration
and fails if any user carries a plaintext password or a hash of the
empty string.

The images live in the flake's `legacyPackages` output rather than
`packages`, so that `nix flake check` does not evaluate them
(GAPI-DIV-068). The commands below are unaffected.

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

These are the formats the flake exposes, and the list is exhaustive: it is
`imageFormats` in `flake.nix`, which is what `nix build .#<format>`
resolves against.

- **iso** - Bootable ISO for USB/CD
- **vm** - QEMU VM with automatic boot
- **qcow** - QCOW2 disk image
- **raw** - Raw disk image
- **vmware** - VMware VMDK
- **lxc** - LXC container
- **lxc-metadata** - the metadata tarball an LXC import needs beside it
- **docker** - Docker image

x86_64-linux only:

- **virtualbox** - VirtualBox OVA

`virtualbox` is separate because its image builder pulls in the
VirtualBox package, which nixpkgs marks x86_64-linux only. Offering it on
aarch64-linux would advertise a target that cannot be built there, which
is the defect `nix flake check --all-systems` exists to catch.

**Adding a format is a one-line change** - append it to `imageFormats`.
nixos-generators supports more than this list, including `vm-nogui`,
`raw-efi`, `proxmox` and the cloud targets (`amazon`, `azure`, `gce`,
`do`). They are not exposed because nobody has built them here, and
**no CI job evaluates the image formats at all** (see below), so an
unbuildable format would be found by whoever next tried it rather than by
a gate. Add one when you intend to build it.

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

## Configuration Files

- `base.nix` - the GAPI configuration every format is built from

That is the whole directory. There is no per-format configuration: every
image comes from `mkImage`, which passes `base.nix` as the only module
and varies `format` alone. So a change to the ISO and a change to the
container are the same change, and a format cannot drift from its
siblings.

## PID 1 Testing

There is no `pid1-test` image, and **the NixOS module exposes no PID 1
option** - it declares `enable`, `package`, `agentsDir`, `configFile`,
`listenAddress`, `certFile`, `keyFile`, `verifyKey`, `logLevel`, `user`,
`group` and `openFirewall`, and nothing else. PID 1 mode is the daemon's
own setting (`supervisor.pid1Mode`, or `--pid1` on `gapid start`), so
reaching it from an image means supplying a config file through
`configFile` rather than setting a module option.

The VM-backed checks are where PID 1 behaviour is actually asserted:

```bash
nix flake check
```

`checks.<system>.module-boot` boots a guest and exercises the module.
Booting a guest is the only way to assert runtime behaviour rather than
merely evaluating it, so a green `nix build` of an image says nothing
about whether the supervisor comes up inside it.

The PID 1 testing guide lives in the goppydae-docs repository.

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

## There is no CI for these images, deliberately

No workflow builds or evaluates an image format. That is the cost of
moving them into `legacyPackages` so that `nix flake check` stops
evaluating them (GAPI-DIV-068), and it is recorded rather than hidden:
**a format that stops evaluating is found by whoever next builds one
locally.**

What CI does keep is `checks.<system>.image-credentials`, which evaluates
this same `base.nix` once and fails if any user carries a plaintext
password or a hash of the empty string. That is the reader GAPI-DIV-069
needs, and it survives the move on purpose.

So if you change `base.nix`, build at least one image locally before
trusting it. Nothing else will.
