# Copyright (c) 2025 Steven Verhelle (enqack)
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.
#
# SPDX-License-Identifier: MPL-2.0

from typing import Protocol, runtime_checkable, Optional, List, Dict, Any, Union
import threading

@runtime_checkable
class InitializeFn(Protocol):
    def __call__(self) -> None: ...

@runtime_checkable
class StartFn(Protocol):
    # Supports optional stop_evt argument
    def __call__(self, stop_evt: Optional[threading.Event] = None) -> None: ...

@runtime_checkable
class StopFn(Protocol):
    def __call__(self) -> None: ...

@runtime_checkable
class ReloadFn(Protocol):
    def __call__(self) -> None: ...

@runtime_checkable
class RestartFn(Protocol):
    def __call__(self) -> None: ...

class Agent(Protocol):
    """
    Protocol describing the optional structure of a GAPI agent module.
    Agents are not required to inherit from this, but doing so provides IDE support.
    """
    ID: str
    NAME: str
    VERSION: str
    TYPE: str
    DESCRIPTION: str
    ENABLED: bool
    DEPS: List[str]
    INTERVAL: Optional[float]
    
    def initialize(self) -> None: ...
    def start(self, stop_evt: Optional[threading.Event] = None) -> None: ...
    def stop(self) -> None: ...
    def reload(self) -> None: ...
    def restart(self) -> None: ...
