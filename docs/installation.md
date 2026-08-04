# Installation Guide

Deploying GAPI beyond a dev shell.

## Contents

- [Quick start](#quick-start)
- [NixOS module](#nixos-module)
- [Flake outputs](#flake-outputs)
- [Systemd unit by hand](#systemd-unit-by-hand)
- [Machine images](#machine-images)
- [Containers](#containers)
- [PID 1](#pid-1)
- [Configuration](#configuration)
- [Verification](#verification)
- [Troubleshooting](#troubleshooting)

## Quick start

### From source, with Nix

```bash
git clone https://github.com/goppydae/gapi.git
```

```bash
cd gapi
```

```bash
nix develop -c mage build
```

Binaries land in `bin/`. `mage build` does not put them on `PATH`;
`mage install` places them in `$GOPATH/bin`.

```bash
nix develop -c mage install
```

### From source, without Nix

Go **1.26.0** or newer (the `go` directive in `go.mod`), plus gcc and
Python 3 if you want the Python ADK. The repo commits a `go.work`
listing `../magelib`, so a lone clone must disable the workspace:

```bash
GOWORK=off go build -o bin/gapid ./cmd/gapid
```

```bash
GOWORK=off go build -o bin/gapictl ./cmd/gapictl
```

Built this way the binaries report version `dev` - the real version is
injected at link time from `VERSION`, which `mage build` does for you.

## NixOS module

The flake exposes `nixosModules.default` (aliased `nixosModules.gapi`)
at the top level, not per-system, so it imports directly:

```nix
{
  inputs.gapi.url = "github:goppydae/gapi";

  outputs = { self, nixpkgs, gapi }: {
    nixosConfigurations.myhost = nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      modules = [
        gapi.nixosModules.default
        {
          services.gapi = {
            enable = true;
            agentsDir = "/var/lib/gapi/agents";
            logLevel = "info";
          };
        }
      ];
    };
  };
}
```

### Options

| Option | Type | Default |
| ------ | ---- | ------- |
| `enable` | bool | `false` |
| `package` | package | `pkgs.callPackage ./package.nix {}` |
| `agentsDir` | path | `/var/lib/gapi/agents` |
| `configFile` | null or path | `null` |
| `listenAddress` | str | `127.0.0.1:14242` |
| `certFile` | path | `/var/lib/gapi/certs/server.crt` |
| `keyFile` | path | `/var/lib/gapi/certs/server.key` |
| `verifyKey` | null or path | `null` |
| `logLevel` | `debug`/`info`/`warn`/`error` | `info` |
| `user` | str | `gapi` |
| `group` | str | `gapi` |
| `openFirewall` | bool | `false` |

`certFile` and `keyFile` are module options; they are written into the
generated config as `transport.tlsCert` and `transport.tlsKey`, which
are the key names the loader actually reads.

The module puts `cfg.package` on `environment.systemPackages`, so
`gapictl` is available once the service is enabled.

With `configFile = null` the module writes `/etc/gapi/config.yaml`,
which is the one search root a release build consults. Setting
`configFile` instead exports `GAPI_CONFIG` to the unit - there is no
`-config` flag on `gapid`.

`openFirewall` opens both the TCP and the UDP port parsed from
`listenAddress`. QUIC is UDP; opening TCP alone achieves nothing.

## Flake outputs

```bash
nix flake show github:goppydae/gapi
```

| Output | What it is |
| ------ | ---------- |
| `nixosModules.default`, `nixosModules.gapi` | the module above |
| `packages.<system>.default`, `.gapi` | the two binaries |
| `legacyPackages.<system>.{iso,vm,qcow,raw,docker,lxc,lxc-metadata,virtualbox,vmware}` | machine images |
| `checks.<system>.module-boot` | a NixOS VM test that boots the module |
| `checks.<system>.image-credentials` | asserts the images ship no default credential |
| `devShells.<system>.default` | the pinned development toolchain |

Systems are `x86_64-linux`, `aarch64-linux` and `aarch64-darwin`. The
image outputs and the checks are Linux-only - on darwin they are absent
rather than broken, so `nix flake check` on a Mac finds nothing to do
instead of failing.

The images sit in `legacyPackages` rather than `packages` because
`nix flake check` names that output without traversing it, and
evaluating seventeen NixOS image fixpoints exhausted the CI runner
(GAPI-DIV-068). It changes nothing about how you build one: `nix build
.#iso` resolves against `packages.<system>` and `legacyPackages.<system>`
in that order. The one visible difference is that `nix flake show`
prints `omitted (use '--legacy' to show)` in place of the image list.

```bash
nix build github:goppydae/gapi
```

```bash
nix flake check
```

## Systemd unit by hand

For non-NixOS hosts. The module above is preferable where it applies.

```ini
[Unit]
Description=GAPI Agent Supervision Framework
After=network.target

[Service]
Type=simple
User=gapi
Group=gapi
ExecStart=/usr/local/bin/gapid
Restart=on-failure
RestartSec=5s
WorkingDirectory=/var/lib/gapi

Environment=GAPI_AGENT_PATH=/var/lib/gapi/agents
Environment=GAPI_CONFIG=/etc/gapi/config.yaml

NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/gapi
ProtectKernelTunables=true
ProtectKernelModules=true
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
RestrictNamespaces=true
LockPersonality=true
RestrictSUIDSGID=true
SystemCallFilter=@system-service ~@privileged ~@resources
SystemCallErrorNumber=EPERM

LimitNOFILE=65536
MemoryMax=2G
TasksMax=256

[Install]
WantedBy=multi-user.target
```

The variables are `GAPI_`-prefixed. `GAPI_AGENTS_DIR` is read by
nothing.

```bash
sudo useradd --system --home /var/lib/gapi --create-home gapi
```

```bash
sudo systemctl enable --now gapi
```

Note that this hardening is incompatible with PID 1 mode, which needs
mount and reboot privileges the sandbox removes.

## Machine images

`nix/generators/base.nix` is a NixOS configuration that imports the
module, enables the service, and installs two example agents. Every
image format builds from it.

| Format | Command |
| ------ | ------- |
| ISO | `nix build .#iso` |
| QEMU VM | `nix build .#vm` |
| QCOW2 | `nix build .#qcow` |
| Raw disk | `nix build .#raw` |
| Docker | `nix build .#docker` |
| LXC | `nix build .#lxc` and `nix build .#lxc-metadata` |
| VirtualBox | `nix build .#virtualbox` |
| VMware | `nix build .#vmware` |

### There is no default login

Every format ships with no usable credential: no password, no
autologin, and SSH set to `PermitRootLogin = "prohibit-password"` with
`PasswordAuthentication = false`. An image is inert until you provision
identity into it - an authorized key baked into your own module, or
cloud-init on the formats that carry it.

This is deliberate and it is enforced. Earlier images carried
`users.users.root.password = "gapi"` in the clear, published in this
repository (GAPI-DIV-069); `checks.<system>.image-credentials` now
evaluates the generator configuration on every `nix flake check` and
fails if any user carries a plaintext password or a hash of the empty
string. Note the residual: any image built from a tree that predates
this change still carries that password, and there is no revocation
path for a baked-in credential.

```bash
nix build .#vm
```

```bash
./result/bin/run-*-vm
```

Inside the guest:

```bash
systemctl status gapi
```

```bash
gapictl agent status
```

The example agents are `heartbeat.py.timer` and `sysinfo.py.service`.
Their metadata is written as real module-level assignments - commented
directives are read by nothing.

### LXC

```bash
nix build .#lxc -o result-rootfs
```

```bash
nix build .#lxc-metadata -o result-metadata
```

```bash
lxc image import ./result-metadata/tarball/*.tar.xz ./result-rootfs/tarball/*.tar.xz --alias gapi-test
```

### Customizing

Edit `nix/generators/base.nix`, or compose a new output in `flake.nix`:

```nix
packages.custom = nixos-generators.nixosGenerate {
  inherit system;
  format = "qcow";
  modules = [
    ./nix/generators/base.nix
    { services.gapi.logLevel = "debug"; }
  ];
};
```

## Containers

A plain OCI image, if you do not want the NixOS-generated one:

```dockerfile
FROM golang:1.26 AS build
WORKDIR /src
COPY . .
ENV GOWORK=off
RUN go build -o /out/gapid ./cmd/gapid && go build -o /out/gapictl ./cmd/gapictl

FROM debian:stable-slim
RUN apt-get update && apt-get install -y python3 && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/gapid /usr/local/bin/gapid
COPY --from=build /out/gapictl /usr/local/bin/gapictl
ENV GAPI_AGENT_PATH=/var/lib/gapi/agents
EXPOSE 14242/udp
ENTRYPOINT ["/usr/local/bin/gapid"]
```

QUIC is UDP. Publishing `14242/tcp` publishes nothing useful.

```bash
docker run -p 14242:14242/udp -v ./agents:/var/lib/gapi/agents gapi
```

Running `gapid` as the container's init:

```bash
docker run --init=false gapi gapid --pid1 --no-early-mounts
```

`--no-early-mounts` matters inside a container: the runtime already owns
`/proc` and `/sys`, and trying to mount them again fails.

## PID 1

PID 1 behaviour is opt-in. `gapid` does not check whether its own pid is
1 - without `--pid1` (or `supervisor.pid1Mode: true`) it is an ordinary
supervisor no matter where it runs.

The only automated path is:

```bash
mage testPid1
```

which runs `gapid` as pid 1 of a rootless podman container. See
[pid1-testing.md](pid1-testing.md) for what it asserts and how to
reproduce it by hand.

For a NixOS guest, the flake's VM checks are the supported route:

```bash
nix build .#checks.x86_64-linux.module-boot
```

## Configuration

See [configuration.md](configuration.md) for every key, and
[config-example.md](config-example.md) for worked examples. In brief:

- A release build reads `/etc/gapi/config.yaml` and nothing else.
- `GAPI_CONFIG` points it elsewhere. There is no `--config` flag.
- Every config key can be set by environment variable: `GAPI_`,
  uppercase, dots to underscores. Environment beats file beats default.
- `transport.insecureSkipVerify` defaults to **true**. Set it to
  `false` before exposing a daemon beyond loopback.
  `supervisor.productionMode` does not do this for you - it gates agent
  signature verification only.

A self-signed certificate for testing:

```bash
openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt -days 365 -nodes -subj "/CN=gapi"
```

## Verification

```bash
systemctl status gapi
```

```bash
gapictl ping
```

```bash
gapictl agent status
```

```bash
journalctl -u gapi -f
```

Deploy a test agent:

```bash
sudo tee /var/lib/gapi/agents/hello.py.service <<'EOF'
ID = "hello"
TYPE = "service"
ENABLED = True

import time


def start():
    while True:
        time.sleep(60)
EOF
```

```bash
sudo systemctl restart gapi
```

```bash
gapictl agent status
```

## Troubleshooting

**The unit will not start.** `journalctl -u gapi -n 50`. A cobra usage
error means a flag that does not exist - `gapid` accepts only
`--runtime-addr`, `--log-level`, `--pid1` and `--no-early-mounts`.

**`gapictl` cannot reach the daemon.** Check the address. The default is
`127.0.0.1:14242`, and the transport is QUIC over UDP - a TCP-only
firewall rule silently blackholes it.

**Agents are not discovered.** Confirm `GAPI_AGENT_PATH` points at
the directory you populated. A Python agent must be named
`<name>.py.service`, `.py.timer` or `.py.socket` - the `.py.` infix is
part of the match, so `hello.service` is not a Python agent. A Go agent
must be an executable file that answers `--describe`.

**An agent is listed but never starts.** Check `ENABLED`. An agent with
`ENABLED = False` is registered and visible but not auto-started;
`gapictl lifecycle start` still works on it.

**Configured TLS is ignored.** Check the key spelling. It is `tlsCert`,
`tlsKey` and `tlsCa`; viper drops unknown keys silently, so
`certFile`/`keyFile` produce a throwaway self-signed certificate rather
than an error.

**Checkpoint operations fail.** `core/checkpoint` shells out to `criu`
and needs `CAP_CHECKPOINT_RESTORE` or `CAP_SYS_ADMIN`. An unprivileged
process gets a capability error even with the binary present.

## Next steps

- [Getting started](getting-started.md) - build and run your first agent
- [Configuration](configuration.md) - every config key and metadata field
- [Development](development.md) - working on GAPI itself
- [PID 1 testing](pid1-testing.md) - running as init
