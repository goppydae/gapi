# Copyright (c) 2025 Steven Verhelle (enqack)
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.
#
# SPDX-License-Identifier: MPL-2.0

# Flake checks for gapi. Each entry is a NixOS VM test: it boots a guest
# and asserts against the running system, not against evaluated Nix.
#
# Run all of them with 'nix flake check'; run one with
# 'nix build .#checks.x86_64-linux.<name>'.
{ pkgs, self }:

# NixOS VM tests only run on Linux. eachDefaultSystem also covers the
# darwin systems, and advertising checks there would make 'nix flake
# check' fail for anyone on a Mac rather than simply find nothing to do.
pkgs.lib.optionalAttrs pkgs.stdenv.hostPlatform.isLinux {
  module-boot = import ./tests/module-boot.nix { inherit pkgs self; };
}
