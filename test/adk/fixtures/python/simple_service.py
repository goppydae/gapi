# Copyright (c) 2025 Steven Verhelle (enqack)
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.
#
# SPDX-License-Identifier: MPL-2.0

# Simple Python service agent for cross-ADK testing

ID = "simple_service"
VERSION = "1.0.0"
TYPE = "service"
DESCRIPTION = "A minimal service agent for testing"

def initialize():
    """Initialize the agent"""
    print("[simple_service] Initialized")

def start(stop_evt=None):
    """Start the agent"""
    print("[simple_service] Started")
    # Service runs until stop event is set
    import time
    if stop_evt:
        while not stop_evt.is_set():
            time.sleep(0.1)
    else:
        # Fallback for old runner or direct execution
        while True:
            time.sleep(1)

def stop():
    """Stop the agent"""
    print("[simple_service] Stopped")
