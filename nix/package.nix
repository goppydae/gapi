{ lib, buildGoModule, gcc, python3, pkg-config, pam }:

buildGoModule rec {
  pname = "gapi";
  version = "0.1.0";
  
  src = lib.cleanSource ../.;
  
  # Set to "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=" for first build
  # Then update with actual hash from error message
  vendorHash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
  
  nativeBuildInputs = [ pkg-config ];
  buildInputs = [ gcc python3 pam ];
  
  # Build both binaries
  subPackages = [ "cmd/gapid" "cmd/gapictl" ];
  
  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
  ];
  
  # Run tests
  checkPhase = ''
    go test -v ./...
  '';
  
  meta = with lib; {
    description = "Agent supervision framework with event-driven daemon management";
    homepage = "https://github.com/goppydae/gapi";
    license = licenses.mit;
    maintainers = with maintainers; [ ];
    platforms = platforms.linux;
  };
}
