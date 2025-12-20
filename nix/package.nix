{ lib, buildGoModule, gcc, python3, pkg-config, pam }:

buildGoModule rec {
  pname = "gapi";
  version = "0.1.0";
  
  # Use cleanSource which respects .gitignore (vendor/ is in .gitignore)
  src = lib.cleanSource ../.;
  
  # Skip vendoring since protobuf files are generated during build
  vendorHash = null;
  
  # Ignore vendor directory during build
  buildFlags = [ "-mod=mod" ];
  
  nativeBuildInputs = [ pkg-config ];
  buildInputs = [ gcc python3 pam ];
  
  # Build both binaries
  subPackages = [ "cmd/gapid" "cmd/gapictl" ];
  
  # Skip tests in Nix build - they work fine in dev shell but fail in build sandbox
  doCheck = false;
  
  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
  ];
  
  # Run tests
  checkPhase = ''
    # Skip integration tests that require binaries to be installed
    go test -v $(go list ./... | grep -v 'test/adk')
  '';
  
  meta = with lib; {
    description = "GoPPydae Agent Programming Interface (GAPI) - Agent supervision framework with event-driven daemon management";
    homepage = "https://github.com/goppydae/gapi";
    license = licenses.mit;
    maintainers = with maintainers; [ ];
    platforms = platforms.linux;
  };
}
