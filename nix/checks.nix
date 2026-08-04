# Copyright (c) 2025 Steven Verhelle (enqack)
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.
#
# SPDX-License-Identifier: MPL-2.0

# Flake checks for gapi. Two kinds, and the difference matters when
# reading a failure:
#
#   module-boot        a NixOS VM test - it boots a guest and asserts
#                      against the running system. Needs KVM and minutes,
#                      so it runs from vm-checks.yml, off the pull-request
#                      path.
#   image-credentials  an EVALUATION-TIME assertion. It builds nothing and
#                      fails while `nix flake check --no-build` is still
#                      evaluating, which is what lets it gate a pull
#                      request in the cheap Flake Build job.
#
# Run all of them with 'nix flake check'; run one with
# 'nix build .#checks.x86_64-linux.<name>'.
{ pkgs, self, nixpkgs }:

# NixOS VM tests only run on Linux. eachDefaultSystem also covers the
# darwin systems, and advertising checks there would make 'nix flake
# check' fail for anyone on a Mac rather than simply find nothing to do.
# The credential check is Linux-only for the same reason one layer down:
# it evaluates a NixOS system, and the image formats it speaks for are
# themselves gated off darwin.
pkgs.lib.optionalAttrs pkgs.stdenv.hostPlatform.isLinux {
  module-boot = import ./tests/module-boot.nix { inherit pkgs self; };
  image-credentials = import ./tests/image-credentials.nix { inherit pkgs nixpkgs; };
}
