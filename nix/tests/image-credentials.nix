# Copyright (c) 2025 Steven Verhelle (enqack)
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.
#
# SPDX-License-Identifier: MPL-2.0

# GAPI-DIV-069. Asserts that the configuration every image format is
# built from carries NO USABLE DEFAULT CREDENTIAL.
#
# THIS IS AN ASSERTION ABOUT THE EVALUATED CONFIGURATION, NOT ABOUT THE
# TEXT OF base.nix. Grepping the source would pass the moment someone
# moved the option into an imported module, a lib.mkIf, or a format
# override, none of which change what the image ships. So the module
# fixpoint is evaluated and the resulting users.users read back.
#
# IT FAILS DURING EVALUATION, not during a build, which is what makes it
# a gate under `nix flake check --no-build`: the throw below is forced
# when the check attribute is evaluated, so the job goes red without
# building anything.
#
# AND IT IS THE READER THAT SURVIVES GAPI-DIV-068. That entry moves the
# image formats into legacyPackages, which `nix flake check` does not
# traverse - which would have deleted the only place in CI this
# configuration was evaluated at all. One evaluation stays, here, and it
# is the one that answers the credential question. Seventeen image
# fixpoints out, one config evaluation per Linux system in.
#
# RESIDUAL, and it is the reason this file says "base configuration"
# rather than "the images": a format module can contribute a credential
# this evaluation cannot see. The iso format demonstrably does - upstream's
# installation-cd profile sets root's initialHashedPassword to "", which
# is why the CI warning that exposed this defect printed for two of the
# seventeen systems and not for all of them. Catching that class needs
# an evaluation per format, which is exactly the cost GAPI-DIV-068 exists
# to remove. The empty-hash cases are asserted here anyway, so a format
# whose credential reaches the base config is caught; one that injects a
# credential only into its own wrapper is not.
{ pkgs, nixpkgs }:

let
  lib = nixpkgs.lib;

  # One NixOS module fixpoint. Not nixos-generators - the format wrapper
  # is irrelevant to the question and is what costs the memory.
  evaluated = lib.nixosSystem {
    modules = [
      { nixpkgs.hostPlatform = pkgs.stdenv.hostPlatform.system; }
      ../generators/base.nix
    ];
  };

  users = evaluated.config.users.users;

  # "Usable" means it lets someone in without the operator having
  # provisioned anything. Two shapes qualify:
  #
  #   - a plaintext password, in either the password or initialPassword
  #     option, which is the shape this repository shipped;
  #   - a hash of the empty string, which crypt(3) accepts as "press
  #     enter" rather than rejecting as "no login". A locked account is
  #     null or "!", and both are fine.
  offendersFor = name: u:
    lib.optional (u.password != null)
      "users.users.${name}.password = ${builtins.toJSON u.password}"
    ++ lib.optional (u.initialPassword != null)
      "users.users.${name}.initialPassword = ${builtins.toJSON u.initialPassword}"
    ++ lib.optional (u.hashedPassword == "")
      "users.users.${name}.hashedPassword = \"\" (empty hash: any password accepted)"
    ++ lib.optional (u.initialHashedPassword == "")
      "users.users.${name}.initialHashedPassword = \"\" (empty hash: any password accepted)";

  offenders = lib.concatLists (lib.mapAttrsToList offendersFor users);

  audited = lib.concatStringsSep " " (lib.attrNames users);
in
if offenders != [ ] then
  throw ''
    nix/generators/base.nix evaluates to a configuration carrying a usable
    default credential, and every advertised image format is built from it
    (GAPI-DIV-069).

    ${lib.concatStringsSep "\n    " offenders}

    An image must ship with no way in until an operator provisions one.
    Remove the credential; do not hash it, do not comment it, and do not
    move it - this check reads the evaluated configuration, not the file.
  ''
else
# A green check that says what it looked at. A gate whose success is
# indistinguishable from its absence is the failure shape this repository
# keeps finding (GAPI-DIV-044), so the audited user list lands in the
# output where a log can show it.
  pkgs.runCommand "gapi-image-credentials"
    {
      inherit audited;
      system = pkgs.stdenv.hostPlatform.system;
    } ''
    {
      echo "no usable default credential in nix/generators/base.nix"
      echo "system:  $system"
      echo "audited: $audited"
    } > $out
  ''
