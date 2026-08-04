# Copyright (c) 2025 Steven Verhelle (enqack)
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.
#
# SPDX-License-Identifier: MPL-2.0

import gapi
import time

def start(stop_evt=None):
    gapi.log.info("Timer agent started")
    
    # Simple timer loop
    import time
    if stop_evt:
        while not stop_evt.is_set():
            gapi.log.info("Tick")
            time.sleep(1)
    else:
        while True:
            gapi.log.info("Tick")
            time.sleep(1)
