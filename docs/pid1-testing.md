# Running and Testing GAPI as PID 1

## The flag is not optional

`gapid` does **not** check whether its own pid is 1. Without `--pid1`
(or `supervisor.pid1Mode: true`) it is an ordinary supervisor no matter
where it is running - booting it as `/sbin/init` without the flag gives
you a process at pid 1 that reaps nothing, mounts nothing, and dies on
the first SIGHUP.

```bash
gapid --pid1
```

```bash
gapid --pid1 --no-early-mounts
```

```yaml
supervisor:
  pid1Mode: true
  noEarlyMounts: false
  watchdog:
    enabled: true
    device: /dev/watchdog
    interval: "10s"
  shutdown:
    gracePeriod: "10s"
```

Use `--no-early-mounts` inside a container: the runtime already owns
`/proc` and `/sys`, and the mount phase is for a real boot.

## What PID 1 mode turns on

| Phase | What happens |
| ----- | ------------ |
| Early mounts | `devtmpfs` on `/dev`, `proc`, `sysfs`, `cgroup2` on `/sys/fs/cgroup`, `tmpfs` on `/run` and `/tmp`. Idempotent over already-mounted targets, and fails closed on the first real error - continuing past a missing `/proc` corrupts everything after it. |
| Subreaper | `PR_SET_CHILD_SUBREAPER`, so orphaned grandchildren reparent here and are reaped with their true wait status. |
| Signal handlers | Installed explicitly. An init has no default dispositions - an unhandled SIGTERM at pid 1 is silently dropped by the kernel. |
| Watchdog | Optional; pets `/dev/watchdog` on an interval. |
| Ordered shutdown | Stop agents, then `sync(2)`, then unmount, then `reboot(2)` with the requested action. |

## Signals

| Signal | PID 1 mode | Normal mode |
| ------ | ---------- | ----------- |
| SIGTERM | graceful shutdown, then poweroff | graceful shutdown |
| SIGINT | graceful shutdown | graceful shutdown |
| SIGHUP | reload | **kills the daemon** |
| SIGUSR1 | debug dump / log rotation | kills the daemon |
| SIGUSR2 | emergency shell hook | kills the daemon |

In normal mode `gapid` notifies only on SIGINT and SIGTERM, so every
other signal keeps Go's default disposition, which is to terminate. Do
not reach for `kill -HUP` to reload a non-init `gapid`; it is a kill.

A shutdown can also be requested over the bus, in either mode:

```bash
gapictl shutdown
```

## The automated path

This is the only PID 1 coverage that runs unattended, and it is what CI
runs:

```bash
mage testPid1
```

It boots `gapid` as pid 1 of a rootless podman container and asserts
two things:

1. **Orphan reaping** - a fixture double-forks, the grandchild
   reparents to `gapid`, and the subreaper loop collects it with its
   *true* exit status (exit 7, wait status 1792). Seeing the real status
   is the proof: nothing else in the chain could have reported it.
2. **Teardown order** - SIGTERM to init produces the graceful stop
   sequence and a clean exit.

Under the hood:

```bash
go test -tags pid1 -timeout 10m -v -run TestPid1 ./test/pid1/
```

The `pid1` build tag is required; without it the package builds to
nothing and the run trivially "passes".

## Interactive testing

### A NixOS guest

The flake's VM checks are the supported route. They boot a real guest
and assert against the running system:

```bash
nix build .#checks.x86_64-linux.module-boot
```

For a guest you can log into, build one of the images. It carries the
NixOS module, which runs `gapid` as a *systemd service*, not as init -
useful for exercising agents and the CLI, not for PID 1 behaviour:

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

```bash
journalctl -u gapi -f
```

### QEMU direct boot

To get `gapid` at pid 1 for real, boot it as the init:

```bash
qemu-system-x86_64 \
  -kernel "$KERNEL" \
  -initrd ./result \
  -append "init=/init console=ttyS0 loglevel=7" \
  -nographic \
  -m 2G
```

Build the initrd with `gapid` symlinked to `/init`, and remember that
`gapid` needs `--pid1`. Bake the flag into a wrapper script used as
`/init`, or put `supervisor.pid1Mode: true` in the config file.
**`RUNTIME_SUPERVISOR_PID1MODE` does not work** - no `supervisor.*` key
can be set from the environment, and the attempt is silently ignored
(GAPI-DIV-038).

### systemd-nspawn

```bash
sudo systemd-nspawn -D /var/lib/machines/gapi-test -b
```

Same caveat: whatever you install as `/sbin/init` must pass `--pid1`,
and should pass `--no-early-mounts` because nspawn has already set up
the filesystem namespace.

## Verification checklist

```bash
pgrep -x gapid
```

Should be `1` when running as init.

```bash
cat /proc/1/cmdline | tr '\0' ' '
```

Confirm `--pid1` is actually there. This is the check that catches the
most common mistake.

```bash
gapictl agent status
```

Note the path - `gapictl status` is not a command.

```bash
gapictl lifecycle stop test
```

```bash
gapictl lifecycle start test
```

```bash
gapictl lifecycle restart test
```

```bash
mount | grep -E 'proc|sysfs|cgroup2'
```

Confirms the early mount phase ran.

```bash
kill -TERM 1
```

Graceful teardown, then poweroff.

## Troubleshooting

**Nothing is reaped and `ps` fills with zombies.** `--pid1` is missing.
Check `/proc/1/cmdline`.

**Mounting fails immediately at boot.** Either the target is already
mounted by something that lied about it, or you are in a container and
want `--no-early-mounts`.

**`gapictl` cannot reach the daemon.** The default address is
`127.0.0.1:14242` and the transport is QUIC over **UDP**. A TCP-only
firewall rule blackholes it silently.

```bash
ss -ulnp | grep 14242
```

**Agents are present but never start.** Check `ENABLED`, then check the
filename - a Python agent must carry the `.py.` infix and one of
`.service`, `.timer`, `.socket`.

```bash
python3 -m py_compile /var/lib/gapi/agents/test.py.service
```

**Where are per-agent logs?** There is no `gapictl logs`. Agent stdout
and stderr are inherited by the supervisor, so they land wherever the
supervisor's output goes - `journalctl -u gapi` under systemd, the
console under init.

**Debug logging.**

```bash
RUNTIME_LOGGING_LEVEL=debug gapid --pid1
```

## Next steps

- [Installation](installation.md) - images, the NixOS module, systemd
- [Configuration](configuration.md) - the supervisor block in full
- [Features](features.md) - what the kernel does
