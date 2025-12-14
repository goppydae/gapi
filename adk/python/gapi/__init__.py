"""
GAPI Python ADK (Agent Development Kit)
Provides optional typing and contracts for building GAPI agents.
"""

from .protocols import Agent, InitializeFn, StartFn, StopFn, ReloadFn, RestartFn
from .schemas import AgentMetadata, AgentDescribe
