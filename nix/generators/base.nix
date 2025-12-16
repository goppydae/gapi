# Base GAPI configuration for nixos-generators
# This configuration is shared across all image formats

{ config, pkgs, lib, modulesPath, ... }:

{
  # Import GAPI module
  imports = [ ../module.nix ];
  
  # Enable GAPI service
  services.gapi = {
    enable = true;
    agentsDir = "/var/lib/gapi/agents";
    logLevel = "info";
    openFirewall = false;  # Override per format if needed
  };
  
  # System configuration
  system.stateVersion = "24.05";
  
  # Basic networking
  networking = {
    hostName = "gapi-test";
    useDHCP = lib.mkDefault true;
    firewall.enable = lib.mkDefault true;
  };
  
  # Enable SSH for remote access
  services.openssh = {
    enable = true;
    settings = {
      PermitRootLogin = "yes";
      PasswordAuthentication = true;
    };
  };
  
  # Set root password for testing (change in production!)
  users.users.root.password = "gapi";
  
  # Install useful packages
  environment.systemPackages = with pkgs; [
    vim
    htop
    curl
    wget
    git
  ];
  
  # Example test agents
  environment.etc = {
    "gapi/agents/heartbeat.py.timer" = {
      text = ''
        # ENABLED = True
        # TYPE = timer
        # SCHEDULE = OnUnitActiveSec=30s
        
        def start():
            import time
            print(f"[{time.strftime('%Y-%m-%d %H:%M:%S')}] Heartbeat from GAPI test system")
      '';
      mode = "0644";
    };
    
    "gapi/agents/sysinfo.py.service" = {
      text = ''
        # ENABLED = True
        # TYPE = service
        
        def start():
            import time
            import platform
            print(f"System: {platform.system()} {platform.release()}")
            print(f"Python: {platform.python_version()}")
            print("GAPI test service running...")
            while True:
                time.sleep(60)
      '';
      mode = "0644";
    };
  };
  
  # Enable serial console for headless testing
  boot.kernelParams = [ "console=ttyS0" ];
  
  # Automatic login on tty1 for easy testing
  services.getty.autologinUser = "root";
}
