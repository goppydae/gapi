# Copyright (c) 2025 Steven Verhelle (enqack)
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.
#
# SPDX-License-Identifier: MPL-2.0

# GAPI-DIV-075. Boots the configuration every advertised image format is
# built from and asserts that the agents it ships are FOUND, not merely
# present.
#
# THAT DISTINCTION IS THE WHOLE POINT OF THIS FILE. module-boot.nix
# asserts `test -d /etc/gapi/agents`, which proves a directory exists and
# says nothing about whether gapid ever searched it - and a directory
# check is exactly what let GAPI-DIV-075 sit unnoticed, because the
# image's agents were present in a tier the image had configured itself
# not to search. An exit that only checked the directory would repeat the
# mistake it was written to catch.
#
# It imports ../generators/base.nix rather than the module, because the
# defect was in the generator: the module was fixed by GAPI-DIV-063 and
# the generator kept the old spelling. Testing the module would have
# passed throughout.
{ pkgs, self }:

pkgs.testers.runNixOSTest {
  name = "gapi-generator-agents";

  nodes.node = { ... }: {
    imports = [ ../generators/base.nix ];
    # The image builds with no credential (GAPI-DIV-069) and the test
    # driver does not need one: it drives the guest through the backdoor,
    # not through login.
  };

  testScript = ''
    node.wait_for_unit("multi-user.target")

    # The agents this image ships land in the operator tier through
    # environment.etc (generators/base.nix).
    node.succeed("test -f /etc/gapi/agents/heartbeat.py.timer")
    node.succeed("test -f /etc/gapi/agents/sysinfo.py.service")

    # THE DELETION, asserted against the EVALUATED unit rather than
    # against the text of base.nix. agentsDir is null now, so the module
    # sets no GAPI_AGENT_PATH at all and the built-in tiers are what
    # gapid searches. Grepping base.nix would pass the moment someone
    # moved the option into an override.
    unit_env = node.succeed("systemctl show gapi.service -p Environment --value")
    assert "GAPI_AGENT_PATH" not in unit_env, (
        "the image sets GAPI_AGENT_PATH again; agentsDir is back "
        "(GAPI-DIV-075): " + unit_env
    )

    # /var/lib is STATE (operator decision 9). The tmpfiles rule that
    # created an agents directory there is gated on agentsDir being set,
    # so with it unset the directory must not exist at all.
    node.fail("test -d /var/lib/gapi/agents")

    # THE DISCRIMINATING ASSERTION: gapid reports the shipped agents as
    # discovered. This is what the directory check could never say.
    # The unit must actually be RUNNING, not merely present. Every image
    # shipped a gapi.service that died on "load cert: no such file or
    # directory" (GAPI-DIV-076), and module-boot.nix could not catch it:
    # it deliberately declines to assert an active state, so a daemon in
    # a permanent restart loop read as "runtime setup this test does not
    # supply".
    node.wait_for_unit("gapi.service")

    rc, status = node.execute("gapictl agent status --tree=false 2>&1")
    print("gapictl agent status rc=%d:\n%s" % (rc, status))
    if rc != 0:
        print(node.execute("journalctl -u gapi.service --no-pager | tail -40")[1])
    assert rc == 0, "gapictl agent status failed (rc=%d):\n%s" % (rc, status)
    for agent in ["heartbeat", "sysinfo"]:
        assert agent in status, (
            "gapid did not discover " + agent + ", which this image ships "
            "into /etc/gapi/agents - the agent directory is populated and "
            "unsearched, which is GAPI-DIV-075's whole subject:\n" + status
        )
  '';
}
