# Copyright (c) 2025 Steven Verhelle (enqack)
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.
#
# SPDX-License-Identifier: MPL-2.0

# gapictl alone, buildable everywhere.
#
# OPERATOR DECISION 10: the control-plane client is cross-platform; the
# daemon is not. The CODE already pays for that - the daemon-side
# packages (criu, libseccomp) are gated behind isLinux precisely so the
# client stays portable - and the PACKAGING withheld it. Before this
# file, `nix eval .#packages.aarch64-darwin --apply builtins.attrNames`
# returned exactly [ "gopy" ]: a code generator, and no control binary
# at all (GAPI-DIV-113).
#
# THE CLIENT WAS NOT MERELY UNPACKAGED, IT WAS UNREACHABLE. nix/package.nix
# is ONE derivation carrying gapid, gapictl and the Python ADK together,
# and it is correctly platforms.linux, because gapid supervises through
# cgroups, the subreaper and CRIU. So the one binary that IS portable was
# bound to a derivation that cannot be. The fix is a second small
# derivation, NOT relaxing that one: widening package.nix would advertise
# a daemon that cannot supervise.
#
# VERIFIED BY CROSS-COMPILING BEFORE THIS FILE EXISTED, and the ordering
# is the point rather than the caution. GOWORK=off CGO_ENABLED=0
# go build -mod=vendor ./cmd/gapictl exits 0 for darwin/arm64 AND
# darwin/amd64. Advertising a package for a platform nobody checked is
# the exact defect GAPI-DIV-068's --all-systems flag was added to catch,
# and goblin produced two more instances of it in one afternoon while
# closing its half of this decision.
#
# WHAT THIS DELIBERATELY DOES NOT CARRY: the Python ADK. That tree is
# built by a gcc link against python3-config, and this repo publishes no
# darwin ADK output for it to come from. So the verbs needing a local
# ADK - `agent build` and `--describe` - degrade on darwin, while the
# control verbs a remote operator actually wants do not touch it. This
# mirrors goblin's nix/goblinctl.nix, which made the same split for the
# same reason. Stating the limit here is the point: whether a darwin ADK
# is buildable at all is a separate question this file does not open,
# and it closes by gapi gaining that output rather than by this file
# guessing at one.
{ lib, buildGoModule }:

buildGoModule rec {
  pname = "gapictl";
  version = lib.fileContents ../VERSION;
  src = ../.;

  # The vendor tree is committed, which is what vendorHash = null selects.
  vendorHash = null;

  # Same reason as nix/package.nix: src carries go.work but not its
  # siblings, so go would enter workspace mode and fail to resolve them.
  env.GOWORK = "off";

  subPackages = [ "cmd/gapictl" ];

  # The REAL injection point, matching nix/package.nix. core/version.GAPIVersion
  # is what the binary reads; main.version does not exist, and an -X
  # naming a symbol nothing declares is dropped in SILENCE, leaving the
  # "dev" placeholder in a binary whose build log looks clean - the
  # unfixed half of GAPI-DIV-007.
  ldflags = [
    "-s"
    "-w"
    "-X github.com/goppydae/gapi/core/version.GAPIVersion=${version}"
  ];

  meta = with lib; {
    description = "gapictl - control-plane client for the GAPI agent runtime";
    homepage = "https://github.com/goppydae/gapi";
    license = licenses.mpl20;
    # Linux AND darwin, unlike the full package. This is the whole point
    # of the file existing.
    platforms = platforms.linux ++ platforms.darwin;
  };
}
