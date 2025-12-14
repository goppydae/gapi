# GAPI PID1 Testing Quick Reference

This guide provides quick commands for testing GAPI as PID1 on NixOS.

## Prerequisites

```bash
# Ensure you're on NixOS
uname -a | grep NixOS

# Verify cgroups v2
mount | grep cgroup2
```

## Method 1: NixOS Container (Fastest)

```bash
# Create container configuration
sudo mkdir -p /etc/nixos/containers
sudo tee /etc/nixos/containers/gapi-test.nix <<'EOF'
{ config, pkgs, ... }:
{
  imports = [ /path/to/gapi/nix/module.nix ];
  
  services.gapi = {
    enable = true;
    agentsDir = "/var/lib/gapi/agents";
  };
  
  # Add test agent
  environment.etc."gapi/agents/test.py.service".text = ''
    # ENABLED = True
    # TYPE = service
    
    def start():
        import time
        print("Test agent running as PID1 child")
        while True:
            time.sleep(60)
  '';
}
EOF

# Create and start container
sudo nixos-container create gapi-test --config-file /etc/nixos/containers/gapi-test.nix
sudo nixos-container start gapi-test

# Check status
sudo nixos-container status gapi-test

# Enter container
sudo nixos-container root-login gapi-test

# Inside container, verify GAPI
systemctl status gapi
gapictl status

# View logs
journalctl -u gapi -f

# Cleanup
exit
sudo nixos-container stop gapi-test
sudo nixos-container destroy gapi-test
```

## Method 2: NixOS VM

```bash
# Build VM
cd /path/to/gapi
nix build .#nixosConfigurations.gapi-vm.config.system.build.vm

# Run VM
./result/bin/run-nixos-vm

# Inside VM
systemctl status gapi
gapictl status
```

## Method 3: systemd-nspawn

```bash
# Create minimal root
sudo mkdir -p /var/lib/machines/gapi-test
sudo debootstrap stable /var/lib/machines/gapi-test

# Copy GAPI binaries
sudo cp bin/gapid /var/lib/machines/gapi-test/sbin/init
sudo cp bin/gapictl /var/lib/machines/gapi-test/usr/bin/

# Boot container
sudo systemd-nspawn -D /var/lib/machines/gapi-test -b

# Cleanup
sudo machinectl poweroff gapi-test
sudo rm -rf /var/lib/machines/gapi-test
```

## Method 4: QEMU Direct Boot

```bash
# Build minimal initrd
nix-build -E '
with import <nixpkgs> {};
let
  gapi = callPackage ./nix/package.nix {};
in
makeInitrd {
  contents = [
    { object = "${gapi}/bin/gapid";
      symlink = "/init";
    }
  ];
}
'

# Get kernel
KERNEL=$(nix-build '<nixpkgs>' -A linux)/bzImage

# Boot with QEMU
qemu-system-x86_64 \
  -kernel $KERNEL \
  -initrd ./result \
  -append "init=/init console=ttyS0 loglevel=7" \
  -nographic \
  -m 2G
```

## Verification Checklist

After starting GAPI as PID1 or in a container:

```bash
# 1. Verify GAPI is running
ps aux | grep gapid

# 2. Check PID
pgrep gapid  # Should be 1 for PID1, or low number in container

# 3. List agents
gapictl status

# 4. Check logs
journalctl -u gapi -n 50  # systemd
# OR
tail -f /var/log/gapi.log  # direct logging

# 5. Test agent lifecycle
gapictl stop test
gapictl start test
gapictl restart test

# 6. Monitor resources
systemd-cgtop  # if systemd is available
top

# 7. Test signal handling
kill -TERM $(pgrep gapid)  # Should gracefully shutdown
kill -HUP $(pgrep gapid)   # Should reload config
```

## Common Issues

### Container won't start

```bash
# Check container status
sudo nixos-container status gapi-test

# View container logs
sudo journalctl -M gapi-test

# Rebuild container
sudo nixos-container update gapi-test
```

### GAPI not responding

```bash
# Check if process exists
ps aux | grep gapid

# Check listening ports
ss -tlnp | grep 4242

# Verify configuration
cat /etc/gapi/config.yaml

# Check permissions
ls -la /var/lib/gapi
```

### Agents not starting

```bash
# Verify agent directory
ls -la /var/lib/gapi/agents

# Check agent syntax
python3 -m py_compile /var/lib/gapi/agents/test.py.service

# View agent-specific logs
gapictl logs test
```

## Performance Testing

```bash
# Create multiple test agents
for i in {1..10}; do
  cat > /var/lib/gapi/agents/test$i.py.service <<EOF
# ENABLED = True
# TYPE = service

def start():
    import time
    while True:
        time.sleep(60)
EOF
done

# Monitor resource usage
watch -n 1 'ps aux | grep gapid'

# Check cgroup limits
cat /sys/fs/cgroup/system.slice/gapi.service/memory.current
cat /sys/fs/cgroup/system.slice/gapi.service/cpu.stat
```

## Debugging

```bash
# Enable debug logging
# Edit /etc/gapi/config.yaml
logging:
  level: debug

# Restart GAPI
sudo systemctl restart gapi

# Watch logs in real-time
journalctl -u gapi -f

# Trace system calls
sudo strace -p $(pgrep gapid)

# Check file descriptors
ls -la /proc/$(pgrep gapid)/fd
```

## Next Steps

- [Full Installation Guide](./INSTALLATION.md)
- [Agent Development](../AGENTS.md)
- [Production Deployment](./DEPLOYMENT.md)
