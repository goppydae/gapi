# Copyright (c) 2025 Steven Verhelle (enqack)
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.
#
# SPDX-License-Identifier: MPL-2.0

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
  
  # Remote access, and NO CREDENTIAL TO REACH IT WITH. GAPI-DIV-069:
  # this file set users.users.root.password = "gapi" in the clear, and
  # since it is the only module every format inherits, all nine
  # advertised images shipped a root account whose password is published
  # in this repository. The mitigation was a comment reading "change in
  # production!", which distinguishes nothing and warns nobody.
  #
  # An image built from here now has no way in until an operator
  # provisions one - an authorized key, or cloud-init on the formats
  # that support it. That is operator decision 2, "no usable default"
  # for identity, and checks.<system>.image-credentials asserts it
  # against the EVALUATED configuration rather than against this text.
  #
  # The two settings below are not merely inert given no password is
  # set. They are what stops the next person who sets one from opening a
  # password path by accident.
  services.openssh = {
    enable = true;
    settings = {
      PermitRootLogin = "prohibit-password";
      PasswordAuthentication = false;
    };
  };

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
      # Real module-level assignments, not comments. Agent metadata is
      # read with getattr on the imported module, so a commented
      # "# TYPE = timer" is silently dropped and the agent never
      # registers as a timer.
      text = ''
        ENABLED = True
        TYPE = "timer"
        SCHEDULE = "OnUnitActiveSec=30s"

        import time


        def start():
            print(f"[{time.strftime('%Y-%m-%d %H:%M:%S')}] Heartbeat from GAPI test system")
      '';
      mode = "0644";
    };
    
    "gapi/agents/sysinfo.py.service" = {
      text = ''
        ENABLED = True
        TYPE = "service"

        import platform
        import time


        def start():
            print(f"System: {platform.system()} {platform.release()}")
            print(f"Python: {platform.python_version()}")
            print("GAPI test service running...")
            while True:
                time.sleep(60)
      '';
      mode = "0644";
    };
  };
  
  # Serial console for headless testing. It carries boot output and a
  # login prompt; it does NOT carry a shell, because there is no
  # credential to answer the prompt with.
  boot.kernelParams = [ "console=ttyS0" ];

  # NO services.getty.autologinUser. It was set to "root", which is the
  # same defect as the plaintext password with no string left to grep
  # for: removing the password while keeping autologin would have left
  # an unauthenticated root shell on tty1 and on the serial console in
  # every one of the nine formats (GAPI-DIV-069).
}
