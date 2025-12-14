from typing import TypedDict, List, Optional

class AgentDescribe(TypedDict, total=False):
    id: str
    name: str
    version: str
    type: str # "service", "timer", "socket"
    language: str
    enabled: bool
    capabilities: List[str]
    description: str
    
    # Dependencies
    requires: List[str]
    wants: List[str]
    wanted_by: List[str]
    required_by: List[str]
    
    # Configuration
    schedule: str
    listen_stream: str
    cpu_limit: str # "0.5" or "500m"
    memory_limit: str # "512MB"
    interval: Optional[float]
    
    # Legacy alias
    deps: List[str]

class AgentMetadata(TypedDict):
    """
    Schema for the full JSON output of --describe
    """
    describe: AgentDescribe
