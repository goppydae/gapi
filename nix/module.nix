{ config, lib, pkgs, ... }:

with lib;

let
  cfg = config.services.gapi;
  
  # Default configuration file
  defaultConfig = pkgs.writeText "gapi-config.yaml" ''
    transport:
      type: quic
      address: ${cfg.listenAddress}
      certFile: ${cfg.certFile}
      keyFile: ${cfg.keyFile}
    
    ${optionalString (cfg.verifyKey != null) ''
    security:
      verifyKey: ${cfg.verifyKey}
    ''}
    
    logging:
      level: ${cfg.logLevel}
      format: json
  '';

in {
  options.services.gapi = {
    enable = mkEnableOption "GAPI agent supervision framework";
    
    package = mkOption {
      type = types.package;
      default = pkgs.callPackage ./package.nix {};
      defaultText = literalExpression "pkgs.gapi";
      description = "GAPI package to use";
    };
    
    agentsDir = mkOption {
      type = types.path;
      default = "/var/lib/gapi/agents";
      description = "Directory containing agent definitions";
    };
    
    configFile = mkOption {
      type = types.nullOr types.path;
      default = null;
      description = ''
        Path to config.yaml. If null, a default configuration will be generated.
      '';
    };
    
    listenAddress = mkOption {
      type = types.str;
      default = "127.0.0.1:4242";
      description = "Address for GAPI to listen on";
    };
    
    certFile = mkOption {
      type = types.path;
      default = "/var/lib/gapi/certs/server.crt";
      description = "Path to TLS certificate file";
    };
    
    keyFile = mkOption {
      type = types.path;
      default = "/var/lib/gapi/certs/server.key";
      description = "Path to TLS key file";
    };
    
    verifyKey = mkOption {
      type = types.nullOr types.path;
      default = null;
      description = "Path to Ed25519 public key for agent verification";
    };
    
    logLevel = mkOption {
      type = types.enum [ "debug" "info" "warn" "error" ];
      default = "info";
      description = "Logging level";
    };
    
    user = mkOption {
      type = types.str;
      default = "gapi";
      description = "User to run GAPI as";
    };
    
    group = mkOption {
      type = types.str;
      default = "gapi";
      description = "Group to run GAPI as";
    };
    
    openFirewall = mkOption {
      type = types.bool;
      default = false;
      description = "Whether to open the firewall for GAPI";
    };
  };

  config = mkIf cfg.enable {
    # Create system user
    users.users.${cfg.user} = {
      isSystemUser = true;
      group = cfg.group;
      description = "GAPI service user";
      home = "/var/lib/gapi";
      createHome = true;
    };
    
    users.groups.${cfg.group} = {};
    
    # Systemd service
    systemd.services.gapi = {
      description = "GAPI Agent Supervision Framework";
      documentation = [ "https://github.com/goppydae/gapi" ];
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];
      
      serviceConfig = {
        Type = "simple";
        User = cfg.user;
        Group = cfg.group;
        ExecStart = "${cfg.package}/bin/gapid ${optionalString (cfg.configFile != null) "-config ${cfg.configFile}"}";
        Restart = "on-failure";
        RestartSec = "5s";
        
        # Working directory
        WorkingDirectory = "/var/lib/gapi";
        
        # Security hardening
        NoNewPrivileges = true;
        PrivateTmp = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        ReadWritePaths = [ "/var/lib/gapi" ];
        ProtectKernelTunables = true;
        ProtectKernelModules = true;
        ProtectControlGroups = true;
        RestrictAddressFamilies = [ "AF_UNIX" "AF_INET" "AF_INET6" ];
        RestrictNamespaces = true;
        LockPersonality = true;
        RestrictRealtime = true;
        RestrictSUIDSGID = true;
        RemoveIPC = true;
        PrivateMounts = true;
        
        # System call filtering
        SystemCallFilter = [ "@system-service" "~@privileged" "~@resources" ];
        SystemCallErrorNumber = "EPERM";
        
        # Resource limits
        LimitNOFILE = 65536;
        LimitNPROC = 512;
        MemoryMax = "2G";
        TasksMax = 256;
      };
      
      environment = {
        GAPI_AGENTS_DIR = cfg.agentsDir;
      };
    };
    
    # Create required directories
    systemd.tmpfiles.rules = [
      "d /var/lib/gapi 0750 ${cfg.user} ${cfg.group} -"
      "d /var/lib/gapi/certs 0750 ${cfg.user} ${cfg.group} -"
      "d ${cfg.agentsDir} 0750 ${cfg.user} ${cfg.group} -"
    ];
    
    # Firewall configuration
    networking.firewall = mkIf cfg.openFirewall {
      allowedTCPPorts = [ (toInt (last (splitString ":" cfg.listenAddress))) ];
      allowedUDPPorts = [ (toInt (last (splitString ":" cfg.listenAddress))) ];
    };
    
    # Use default config if none provided
    environment.etc."gapi/config.yaml" = mkIf (cfg.configFile == null) {
      source = defaultConfig;
    };
  };
}
