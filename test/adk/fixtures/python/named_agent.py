# Copyright (c) 2025 Steven Verhelle (enqack)
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.
#
# SPDX-License-Identifier: MPL-2.0

# Python agent declaring a NAME distinct from its ID.
#
# Every other fixture leaves NAME unset, so the reported name defaults to
# the id and a describe payload that lost the declared name would still
# look plausible. This one makes the two values impossible to confuse.

ID = "named_agent"
NAME = "Named Agent"
VERSION = "1.0.0"
TYPE = "service"


def initialize():
    """Initialize the agent"""
    pass


def start():
    """Start the agent"""
    pass


def stop():
    """Stop the agent"""
    pass
