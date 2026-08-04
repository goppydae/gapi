# Copyright (c) 2025 Steven Verhelle (enqack)
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.
#
# SPDX-License-Identifier: MPL-2.0

"""
GAPI Python ADK (Agent Development Kit)
Provides optional typing and contracts for building GAPI agents.
"""

from .protocols import Agent, InitializeFn, StartFn, StopFn, ReloadFn, RestartFn
from .schemas import AgentMetadata, AgentDescribe
from .agent import capability
