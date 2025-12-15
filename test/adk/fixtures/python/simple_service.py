# Simple Python service agent for cross-ADK testing

ID = "simple_service"
VERSION = "1.0.0"
TYPE = "service"
DESCRIPTION = "A minimal service agent for testing"

def initialize():
    """Initialize the agent"""
    print("[simple_service] Initialized")

def start():
    """Start the agent"""
    print("[simple_service] Started")
    # Service runs indefinitely
    import time
    while True:
        time.sleep(1)

def stop():
    """Stop the agent"""
    print("[simple_service] Stopped")
