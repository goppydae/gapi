# Copyright (c) 2025 Steven Verhelle (enqack)
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.
#
# SPDX-License-Identifier: MPL-2.0

{ lib, buildGoModule, gcc, python3, pkg-config, pam, makeWrapper }:

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
  
  nativeBuildInputs = [ pkg-config makeWrapper ];
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

  # THE PYTHON ADK IS PART OF THE PRODUCT, NOT A DEVELOPMENT CONVENIENCE
  # (GAPI-DIV-077). A Python agent is described by running the ADK runner
  # against it, so a gapid with no runner cannot discover a *.py.service
  # at all - and core/agentmgr/discovery.go SWALLOWS that failure, so the
  # daemon reports "agent discovery complete count=0" and looks healthy.
  # Every advertised image shipped exactly that: two Python agents in
  # /etc/gapi/agents and nothing able to read them.
  #
  # resolvePyRunner (core/supervisor/lifecycle_handlers.go:200) looks in
  # three places - GAPI_PY_RUNNER, then adk/python/agent/runner.py NEXT
  # TO THE BINARY, then the same path RELATIVE TO THE CWD. The last is
  # what a systemd unit gets, and it resolves against / on a booted
  # system, which is why this was invisible in a checkout and fatal in an
  # image: in a dev tree the cwd fallback happens to hit.
  #
  # Fixed in the PACKAGE rather than in module.nix so that every consumer
  # of the derivation gets a working runner, not only NixOS. The wrapper
  # sets the override explicitly instead of relying on the
  # next-to-the-binary probe: $out/bin holds binaries, and burying a
  # python tree in it to satisfy a path probe is the kind of layout that
  # gets tidied away by someone who does not know it is load-bearing.
  # gapid only: gapictl does not discover agents, and giving it a python
  # on PATH it never calls would be scope the wrapper cannot justify.
  postInstall = ''
    mkdir -p $out/share/gapi
    cp -r adk/python $out/share/gapi/python

    wrapProgram $out/bin/gapid \
      --set-default GAPI_PY_RUNNER $out/share/gapi/python/agent/runner.py \
      --prefix PATH : ${lib.makeBinPath [ python3 ]}
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
