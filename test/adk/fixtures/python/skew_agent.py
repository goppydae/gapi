# Copyright (c) 2025 Steven Verhelle (enqack)
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.
#
# SPDX-License-Identifier: MPL-2.0

# An agent that reports a protobuf contract no daemon in this tree can
# share, so the daemon's mismatch report has something to detect
# (GAPI-DIV-127).
#
# THE MISMATCH IS FORCED THROUGH THE REAL DESCRIBE PATH rather than by
# writing a hash into a fixture's metadata. The runner builds
# "schema_hash" from adk.SchemaHash(), so overriding the ADK's value here
# is the only way to make discovery read a skewed agent the same way it
# would read a genuinely stale binary. A fixture that declared the field
# itself would prove the test could type a string.
#
# THE IMPORT IS DELIBERATELY UNGUARDED. Without the native extension the
# stub's SchemaHash() returns "", the daemon is silent in that unknown
# direction by design, and the test would go red reporting "no warning"
# for a missing toolchain. A loud ImportError names the real cause; the
# harness sets ADK_REJECT_DUMMY for the same reason.

from gapi.native import adk as _adk

ID = "skew_agent"
VERSION = "1.0.0"
TYPE = "service"
DESCRIPTION = "Agent reporting a contract hash this daemon cannot share"

# All zeroes, and the length matters: 64 hex characters is the shape of a
# real BLAKE3 digest, so this exercises the comparison rather than a
# length check somewhere upstream deciding the field is malformed.
FORCED_SCHEMA_HASH = "0" * 64

_adk.SetSchemaHash(FORCED_SCHEMA_HASH)

state = {"running": False}


def start():
    """Start the agent"""
    state["running"] = True
    import time
    while state["running"]:
        time.sleep(0.5)


def stop():
    """Stop the agent"""
    state["running"] = False
