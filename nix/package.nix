# Copyright (c) 2025 Steven Verhelle (enqack)
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.
#
# SPDX-License-Identifier: MPL-2.0

{ lib, buildGoModule, gcc, python3, pkg-config, pam }:

buildGoModule rec {
  pname = "gapi";
  # Read from the root VERSION file, which is the single source of version
  # truth (magelib.Version reads the same file). Hardcoding it here meant
  # the packaged binary stamped 0.1.0 while every mage build stamped
  # whatever VERSION said - the same class of drift as GAPI-DIV-007, just
  # on the Nix side.
  version = lib.fileContents ../VERSION;

  # cleanSource filters VCS and editor files - it does NOT read
  # .gitignore. vendor/ is tracked (2203 files) and is what this build
  # consumes, which is what vendorHash = null selects.
  src = lib.cleanSource ../.;
  vendorHash = null;

  # The committed go.work lists ../magelib, which src does not carry into
  # the sandbox, so go would enter workspace mode and fail to resolve it.
  # GOWORK=off is the lone-clone contract CI already uses. The previous
  # buildFlags = [ "-mod=mod" ] made this worse: -mod is illegal in
  # workspace mode, so the build could not succeed either way.
  env.GOWORK = "off";
  
  nativeBuildInputs = [ pkg-config ];
  buildInputs = [ gcc python3 pam ];
  
  # Build both binaries
  subPackages = [ "cmd/gapid" "cmd/gapictl" ];
  
  # Skip tests in Nix build - they work fine in dev shell but fail in build sandbox
  doCheck = false;
  
  # Stamp the real injection point. core/version.GAPIVersion is what
  # both binaries read (core/version/version.go); main.version does not
  # exist, so the previous flag silently left the "dev" placeholder in
  # place - the unfixed half of GAPI-DIV-007, which was closed on the
  # Magefile change alone.
  ldflags = [
    "-s"
    "-w"
    "-X github.com/goppydae/gapi/core/version.GAPIVersion=${version}"
  ];
  
  # Run tests
  checkPhase = ''
    # Skip integration tests that require binaries to be installed
    go test -v $(go list ./... | grep -v 'test/adk')
  '';
  
  meta = with lib; {
    description = "GoPPydae Agent Process Infrastructure (GAPI) - Agent supervision framework with event-driven daemon management";
    homepage = "https://github.com/goppydae/gapi";
    # MPL-2.0, per the root LICENSE file and the README. This said mit,
    # which is not the licence this code ships under.
    license = licenses.mpl20;
    maintainers = with maintainers; [ ];
    platforms = platforms.linux;
  };
}
