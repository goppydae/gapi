# Copyright (c) 2025 Steven Verhelle (enqack)
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.
#
# SPDX-License-Identifier: MPL-2.0

# Proves the flake's nixosModules output is importable and evaluates
# into a bootable system carrying a real gapi package.
#
# This is also the regression test for GAPI-DIV-033, where the module
# could not work at all: ExecStart passed a -config flag gapid does not
# define, so the unit failed to start whenever configFile was set, and
# the unit environment set GAPI_AGENTS_DIR, which nothing reads.
#
# Deliberately does NOT assert that gapid reaches an active state under
# every configuration: that depends on runtime setup this test does not
# supply. What it asserts is that the unit is wired to real things - a
# buildable binary, an argument list the binary accepts, and the
# environment variables the loader actually reads.
{ pkgs, self }:

pkgs.testers.runNixOSTest {
  name = "gapi-module-boot";

  nodes.node = { ... }: {
    imports = [ self.nixosModules.default ];
    services.gapi.enable = true;
    services.gapi.agentsDir = "/var/lib/gapi/agents";
    # certFile/keyFile are null by default since GAPI-DIV-076, and the
    # config assertions below are about the SPELLING the loader reads
    # (tlsCert, not certFile - viper drops unknown keys silently). That
    # question only exists when a certificate is configured at all, so
    # this node configures one. The paths need not resolve: nothing here
    # starts the daemon.
    services.gapi.certFile = "/var/lib/gapi/certs/server.crt";
    services.gapi.keyFile = "/var/lib/gapi/certs/server.key";
  };

  testScript = ''
    node.wait_for_unit("multi-user.target")

    # The unit exists and runs the packaged binary.
    node.succeed("systemctl cat gapi.service")
    exec_start = node.succeed(
        "systemctl show gapi.service -p ExecStart --value"
    )
    assert "gapid" in exec_start, (
        "gapi.service ExecStart does not run gapid: " + exec_start
    )

    # The daemon lives behind an explicit start verb (cli-contract.md,
    # GAPI-DIV-057). A bare ExecStart no longer boots anything: the root
    # carries no RunE, so it prints help and exits 1, and the unit would
    # fail on every start with no other signal.
    assert "gapid start" in exec_start, (
        "ExecStart does not use the start verb; a bare gapid prints help "
        "and exits nonzero: " + exec_start
    )

    # No -config flag. gapid's start verb defines --listen-addr, --pid1
    # and --no-early-mounts locally, plus the shared persistent daemon
    # set; cobra rejects anything else, so a stray -config here means the
    # unit can never start.
    assert "-config" not in exec_start, (
        "ExecStart passes -config, which gapid does not accept: " + exec_start
    )

    # The agents directory reaches the loader through the variable it
    # actually reads. GAPI_AGENTS_DIR was set here for a long time and
    # consumed by nothing.
    #
    # This node sets agentsDir explicitly, so AGENT_PATH must appear.
    # Since GAPI-DIV-063 that variable ADDS precedence rather than
    # replacing the search path, so it is an extra directory and not the
    # only one.
    unit_env = node.succeed("systemctl show gapi.service -p Environment --value")
    assert "GAPI_AGENT_PATH" in unit_env, (
        "GAPI_AGENT_PATH is not set on the unit, so agentsDir does nothing: "
        + unit_env
    )
    assert "GAPI_AGENTS_DIR" not in unit_env, (
        "GAPI_AGENTS_DIR is set but read by no code: " + unit_env
    )

    # The operator tier exists without anyone configuring it. /etc/gapi/
    # agents is a built-in search path, so an admin must have somewhere
    # to drop an agent without first learning which directory is read.
    node.succeed("test -d /etc/gapi/agents")

    # /var/lib is STATE. Agents are executable payload and no longer
    # default to living there (GAPI-DIV-063).
    node.succeed("test -d /var/lib/gapi/certs")

    # The binary is real and reports a stamped version rather than the
    # 'dev' placeholder (GAPI-DIV-007).
    version = node.succeed("gapid version")
    print(version)
    assert "Runtime Core: dev" not in version, (
        "the packaged binary carries no version stamp:\n" + version
    )

    # The generated config uses the keys the loader reads. certFile and
    # keyFile were silently ignored by viper, which downgraded the
    # daemon to a throwaway self-signed certificate.
    cfg = node.succeed("cat /etc/gapi/config.yaml")
    assert "tlsCert" in cfg, "generated config does not use tlsCert:\n" + cfg
    assert "certFile" not in cfg, (
        "generated config still uses certFile, which viper drops:\n" + cfg
    )
  '';
}
