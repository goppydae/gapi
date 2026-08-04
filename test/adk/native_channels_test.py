# Copyright (c) 2025 Steven Verhelle (enqack)
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.
#
# SPDX-License-Identifier: MPL-2.0

import sys
import os
import threading
import time

# Ensure adk/python is in path
sys.path.append(os.path.abspath(os.path.join(os.path.dirname(__file__), '../../adk/python')))

try:
    from gapi.native import adk
except ImportError as e:
    print(f"Failed to import gapi.native.adk: {e}")
    sys.exit(1)

def sender(mgr):
    time.sleep(1) 
    print("Sender: Sending 'hello from adk'...")
    mgr.Send("hello from adk")
    print("Sender: Sent")

def main():
    print("Creating ChannelManager...")
    mgr = adk.NewChannelManager()
    
    print("Starting sender thread...")
    t = threading.Thread(target=sender, args=(mgr,))
    t.start()
    
    print("Receiver: Waiting for message (timeout=5s)...")
    try:
        msg = mgr.Receive(5)
        print(f"Receiver: Got message: '{msg}'")
        if msg != "hello from adk":
            print("FAIL: message mismatch")
            sys.exit(1)
    except Exception as e:
        print(f"Receiver: Error: {e}")
        sys.exit(1)
        
    t.join()
    print("SUCCESS: ADK Native Channel communication verified")

if __name__ == "__main__":
    main()
