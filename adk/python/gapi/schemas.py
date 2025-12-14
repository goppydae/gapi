from typing import TypedDict, List, Optional

class AgentDescribe(TypedDict, total=False):
    id: str
    name: str
    version: str
    type: str # "service", "job", "system"
    language: str 
    enabled: bool
    capabilities: List[str]
    deps: List[str]
    description: str
    interval: Optional[float]

class AgentMetadata(TypedDict):
    """
    Schema for the full JSON output of --describe
    """
    describe: AgentDescribe
