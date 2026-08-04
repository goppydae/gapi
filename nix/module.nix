# Copyright (c) 2025 Steven Verhelle (enqack)
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.
#
# SPDX-License-Identifier: MPL-2.0

{ config, lib, pkgs, ... }:

with lib;

let
  cfg = config.services.gapi;
  
  # Default configuration file.
  #
  # The TLS keys are tlsCert/tlsKey - the mapstructure tags on
  # core/config.TransportConfig. Viper drops unknown keys silently, so
  # the previous certFile/keyFile spelling meant the daemon ignored the
  # configured certificate and generated a throwaway self-signed one
  # instead: a silent downgrade rather than an error. This note lives
  # here rather than in the emitted YAML, which operators read.
  defaultConfig = pkgs.writeText "gapi-config.yaml" ''
    transport:
      type: quic
      address: ${cfg.listenAddress}
      tlsCert: ${cfg.certFile}
      tlsKey: ${cfg.keyFile}
    
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
      type = types.nullOr types.path;
      default = null;
      description = ''
        An ADDITIONAL agent directory, prepended to the built-in search
        path. Leave null unless agents live somewhere non-standard.

        Operator-authored agents belong in /etc/gapi/agents, which is a
        built-in tier and needs no option at all. /var/lib/gapi is state
        (the database, certificates) and is deliberately NOT an agent
        directory: agents are executable payload, not variable state.

        This used to default to /var/lib/gapi/agents and was passed as
        GAPI_AGENT_PATH, which REPLACED the whole search path - so every
        tier below it was dead on a packaged install (GAPI-DIV-063).
      '';
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
      # Matches the runtime default, which core/config takes from
      # core/product.DefaultControlAddr (GAPI-DIV-071) rather than from a
      # literal, so goblind does not inherit gapi's port. 4242 was never
      # the port.
      default = "127.0.0.1:14242";
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
    # Put gapid and gapictl on PATH. Enabling the service without them
    # leaves an operator with a running daemon and no way to talk to it
    # short of digging the store path out of the unit. goblin's module
    # does the same.
    environment.systemPackages = [ cfg.package ];

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
        # gapid takes no -config flag: cmd/gapid/gapid.go registers only
        # --runtime-addr, --log-level, --pid1 and --no-early-mounts, and
        # cobra rejects anything else, so passing one made the unit fail
        # to start whenever configFile was set. The config override is an
        # environment variable (core/config/config.go reads
        # GAPI_CONFIG), set below.
        ExecStart = "${cfg.package}/bin/gapid start";
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
      
      # GAPI_-prefixed, because that is what the loader reads.
      # GAPI_AGENTS_DIR was set here and consumed by nothing
      # (core/config/agent_paths.go reads GAPI_AGENT_PATH), so
      # services.gapi.agentsDir was a no-op and its default was not even
      # on the search path.
      #
      # AGENT_PATH is now ADDITIVE, so setting it adds precedence rather
      # than discarding /etc/gapi/agents and the rest. It is set only
      # when the operator asked for an extra directory; the built-in
      # tiers cover the normal case unaided.
      environment = optionalAttrs (cfg.agentsDir != null) {
        GAPI_AGENT_PATH = toString cfg.agentsDir;
      } // optionalAttrs (cfg.configFile != null) {
        GAPI_CONFIG = toString cfg.configFile;
      };
    };
    
    # Create required directories
    systemd.tmpfiles.rules = [
      # /var/lib is STATE: the database and certificates. Agents are
      # executable payload and live on the search path instead.
      "d /var/lib/gapi 0750 ${cfg.user} ${cfg.group} -"
      "d /var/lib/gapi/certs 0750 ${cfg.user} ${cfg.group} -"

      # The operator tier, created so an admin has somewhere to put an
      # agent without first discovering which directory is searched.
      "d /etc/gapi 0755 root root -"
      "d /etc/gapi/agents 0755 root root -"
    ] ++ optionals (cfg.agentsDir != null) [
      "d ${toString cfg.agentsDir} 0750 ${cfg.user} ${cfg.group} -"
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
