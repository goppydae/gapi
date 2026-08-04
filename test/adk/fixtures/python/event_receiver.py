# Copyright (c) 2025 Steven Verhelle (enqack)
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.
#
# SPDX-License-Identifier: MPL-2.0

import gapi

def start(stop_evt=None):
    gapi.log.info("Receiver started")
    
    # Subscribe to events from the emitter
    # Using adk helper or direct subscription if available
    # Actually runner doesn't pass ctx with subscribe method easily to start?
    # Runner initializes ADK globally.
    # We can use gapi.subscribe if it wraps ADK, but runner doesn't seem to expose a 'gapi' module with subscribe.
    # runner.py imports gapi.native.adk.
    # If the fixture imports gapi, it might be importing the package from sys.path?
    # runner.py appends ".." to sys.path.
    
    # Let's assume gapi.native.adk is available via 'import gapi.native.adk as adk' or similar?
    # But fixtures import 'gapi'.
    
    # For now, just keep the loop. 
    # Subscription logic might require gapi package reference.
    
    import time
    if stop_evt:
        while not stop_evt.is_set():
            time.sleep(1)
    else:
        while True:
            time.sleep(1)

def on_event(event):
    gapi.log.info(f"Received event: {event.data}")
