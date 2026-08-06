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
  # tlsCert/tlsKey appear ONLY when an operator provisioned a
  # certificate. Emitting them unconditionally is what made every image
  # boot into a crash loop (GAPI-DIV-076): certFile and keyFile defaulted
  # to paths under /var/lib/gapi/certs, systemd.tmpfiles created the
  # DIRECTORY and nothing ever created the FILES, so the daemon took
  # transport/factory.go's "a cert is configured" branch and died on
  # "load cert: no such file or directory". Naming a path is not
  # provisioning one.
  #
  # Left out, the daemon's own fallback runs and generates a self-signed
  # certificate, warning loudly that it is not for production - which is
  # the honest posture for an image that also ships no credential
  # (GAPI-DIV-069). An operator who sets certFile/keyFile gets exactly
  # what they named.
  tlsLines = optionalString (cfg.certFile != null && cfg.keyFile != null) ''
      tlsCert: ${toString cfg.certFile}
      tlsKey: ${toString cfg.keyFile}
  '';

  defaultConfig = pkgs.writeText "gapi-config.yaml" ''
    transport:
      type: quic
      address: ${cfg.listenAddress}
    ${tlsLines}

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
      default = "127.0.0.1:29979";
      description = "Address for GAPI to listen on";
    };
    
    # NULL BY DEFAULT, like agentsDir and verifyKey below, and for the
    # same reason: a non-null default here named a file the module never
    # created, and the daemon cannot tell "the operator gave me this
    # path" from "the module guessed it" (GAPI-DIV-076).
    certFile = mkOption {
      type = types.nullOr types.path;
      default = null;
      description = ''
        Path to a TLS certificate the operator has provisioned. Leave
        null to let the daemon generate a self-signed certificate at
        startup, which it warns about and which is not for production.
        /var/lib/gapi/certs is created for you to put one in.
      '';
    };

    keyFile = mkOption {
      type = types.nullOr types.path;
      default = null;
      description = ''
        Path to the private key for certFile. Both must be set for the
        daemon to use them; either left null falls back to a generated
        self-signed certificate.
      '';
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
        # gapid takes no -config flag: pkg/cli.RegisterDaemonFlags binds
        # the root's persistent set (--id, the four --log-* names,
        # --metrics-addr and the three --tls-* names) and pkg/cli adds
        # --listen-addr, --pid1 and --no-early-mounts to `start`. Cobra
        # rejects anything else, so passing one made the unit fail to
        # start whenever configFile was set. The config override is an
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
