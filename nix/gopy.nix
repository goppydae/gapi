# Copyright (c) 2025 Steven Verhelle (enqack)
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.
#
# SPDX-License-Identifier: MPL-2.0

# gopy, the Python binding code generator, built from the pinned tools
# module (GAPI-DIV-096).
#
# WHY THIS EXISTS RATHER THAN A SHELL HOOK. The devshell used to build
# gopy at entry with `(cd tools/gopy && go build ...)` against whatever
# module cache the developer happened to have. That is fine on a warm
# laptop and impossible in a Nix sandbox, which has no network and no
# shared cache - so the packaging work GAPI-DIV-085 needs, where the
# derivation must generate the binding itself, had nowhere to get the
# tool. Every other tool the build shells out to comes from the flake;
# this one now does too.
#
# The pin is unchanged and still lives in tools/gopy/go.mod. What moves
# is only WHERE the build happens: a fixed-output derivation fetches the
# module graph once against go.sum, and every consumer - the shell, a
# package, CI - gets the same binary from the same inputs.
{ lib, buildGoModule }:

buildGoModule {
  pname = "gopy";

  # gopy's own version, not gapi's. This derivation builds a pinned
  # third-party tool; stamping it with the kernel's VERSION would make
  # the store path move on every gapi release for a binary that did not
  # change.
  version = "0.4.10";

  # The TOOLS module, not the repository root. tools/gopy exists because
  # gopy cannot be a dependency of the main module: it is a build-time
  # tool whose own dependencies (an older x/tools) would otherwise be
  # dragged into the kernel's module graph. cleanSource keeps the
  # derivation from rebuilding on unrelated repository churn.
  src = lib.cleanSource ../tools/gopy;

  # Recomputed whenever tools/gopy/go.sum changes, which is the only
  # thing that can change what this fetches. `nix build` prints the
  # correct value on mismatch.
  vendorHash = "sha256-NRdksWpaCk/vu3XrJfD0M13oZhGawcpT1MDdRMvsL5Q=";

  # gopy's main package is a DEPENDENCY of this module, not a package
  # inside it - tools/gopy holds only go.mod, go.sum and the tools.go
  # import anchor that keeps it in the graph. buildGoModule's default
  # phase builds ./..., which here is nothing, so the build names the
  # external package explicitly against the vendored tree.
  buildPhase = ''
    runHook preBuild
    go build -o $GOPATH/bin/gopy github.com/go-python/gopy
    runHook postBuild
  '';

  # Nothing in tools/gopy is testable: it declares no packages of its
  # own, and running gopy's upstream suite would test a pinned release
  # this repository does not maintain.
  doCheck = false;

  meta = with lib; {
    description = "Go bindings generator for Python, pinned for the GAPI Python ADK";
    homepage = "https://github.com/go-python/gopy";
    license = licenses.bsd3;
    mainProgram = "gopy";
    # Deliberately NOT restricted to Linux. The daemon is Linux-only;
    # this is a code generator, and the darwin dev shell needs it for
    # exactly the same reason the Linux one does.
    platforms = platforms.unix;
  };
}
