# Copyright (c) 2025 Steven Verhelle (enqack)
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.
#
# SPDX-License-Identifier: MPL-2.0

# Python agent with full lifecycle and capabilities

ID = "lifecycle_agent"
VERSION = "1.0.0"
TYPE = "service"
DESCRIPTION = "Agent demonstrating full lifecycle support"

state = {"initialized": False, "running": False}

def initialize():
    """Initialize the agent"""
    state["initialized"] = True
    print("[lifecycle_agent] Initialized")

def start():
    """Start the agent"""
    if not state["initialized"]:
        raise RuntimeError("Cannot start before initialization")
    state["running"] = True
    print("[lifecycle_agent] Started")
    import time
    while state["running"]:
        time.sleep(0.5)

def stop():
    """Stop the agent"""
    state["running"] = False
    print("[lifecycle_agent] Stopped")

def reload():
    """Reload configuration"""
    print("[lifecycle_agent] Reloaded")
