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
  # THE GO ADK IS PART OF THE PRODUCT TOO, FOR THE SAME REASON
  # (GAPI-DIV-092). `gapictl agent build` assembles the author's file with
  # a generated main and compiles the result, and the generated main
  # imports the ADK runtime. Before this, that import could only resolve
  # inside a checkout, so an operator who INSTALLED gapi got
  # `go: go.mod file not found` from the exact command `agent new` had
  # just told them to run.
  #
  # The staged build takes this tree as a local `replace` target, so
  # nothing here is fetched and the whole build works air-gapped. That is
  # why the SOURCE ships rather than a version to resolve: an installed
  # host may have no proxy, and a Go agent is the form this architecture
  # treats as canonical.
  #
  # adk/go/agent only - NOT adk/go, whose exported surface gopy binds into
  # the Python extension and which pulls in the kernel. The runtime is
  # stdlib-only, which is what makes a five-file module enough.
  #
  # The go directive is copied from the kernel's own go.mod rather than
  # written here, so the shipped module cannot claim a toolchain the
  # source was not written against.
  #
  # gapictl is wrapped, unlike for the Python runner: gapictl is the
  # binary that reads this, and setting the override explicitly beats
  # relying on the ../share path probe surviving someone's tidy-up. The
  # probe stays in resolveGoADK for installs that are not this derivation.
  # No Go toolchain is added to PATH: pinning the compiler an operator
  # builds their agents with is not this package's decision, and it would
  # put the whole toolchain in gapi's runtime closure.
  postInstall = ''
    mkdir -p $out/share/gapi
    cp -r adk/python $out/share/gapi/python

    mkdir -p $out/share/gapi/go/agent
    for f in adk/go/agent/*.go; do
      case "$f" in
        *_test.go) continue ;;
      esac
      cp "$f" $out/share/gapi/go/agent/
    done
    goDirective=$(sed -n 's/^go \(.*\)$/\1/p' go.mod | head -n 1)
    if [ -z "$goDirective" ]; then
      echo "go.mod declares no go directive; the shipped ADK module would be invalid" >&2
      exit 1
    fi
    printf 'module github.com/goppydae/gapi/adk/go\n\ngo %s\n' "$goDirective" \
      > $out/share/gapi/go/go.mod

    wrapProgram $out/bin/gapid \
      --set-default GAPI_PY_RUNNER $out/share/gapi/python/agent/runner.py \
      --prefix PATH : ${lib.makeBinPath [ python3 ]}

    wrapProgram $out/bin/gapictl \
      --set-default GAPI_GO_ADK $out/share/gapi/go
  '';

  # THE PACKAGING HALF OF GAPI-DIV-092 NEEDS ITS OWN GATE.
  #
  # pkg/cli's tests point GAPI_GO_ADK at the checkout, so they prove the
  # CODE works and say nothing about whether this derivation SHIPS what
  # that code looks for. Delete the postInstall lines above and every Go
  # test still passes while an installed operator is broken again - which
  # is exactly the shape of GAPI-DIV-085, where a packaged daemon
  # advertised Python agents it had no runner to start.
  #
  # So the assertion lives where the artifact is built. It runs in the
  # Flake Build job, and it fails the derivation rather than a test that
  # was never looking.
  doInstallCheck = true;
  installCheckPhase = ''
    runHook preInstallCheck

    for required in \
      share/gapi/go/go.mod \
      share/gapi/go/agent/run.go \
      share/gapi/python/agent/runner.py
    do
      if [ ! -f "$out/$required" ]; then
        echo "packaged tree is missing $required - an installed operator cannot build or run an agent" >&2
        exit 1
      fi
    done

    # A go.mod naming the wrong module compiles to a confusing import
    # error at agent-build time, on the operator's machine, not here.
    if ! grep -qx 'module github.com/goppydae/gapi/adk/go' $out/share/gapi/go/go.mod; then
      echo "shipped ADK go.mod does not declare the ADK module path:" >&2
      cat $out/share/gapi/go/go.mod >&2
      exit 1
    fi

    # Tests are not compiled into an agent, and shipping them would make
    # the provenance hash move for edits that cannot change a binary.
    #
    # find, not `ls <glob>`: the stdenv sets nullglob, so a glob that
    # matches nothing expands to NOTHING and `ls` then lists the current
    # directory and succeeds. The first version of this check was written
    # that way and reported a leak on a clean tree.
    leaked=$(find $out/share/gapi/go/agent -name '*_test.go' -print -quit)
    if [ -n "$leaked" ]; then
      echo "ADK test files leaked into the packaged tree: $leaked" >&2
      exit 1
    fi

    runHook postInstallCheck
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
