# Python agent with capability decorators

ID = "capabilities_agent"
VERSION = "1.0.0"
TYPE = "service"

def initialize():
    """Initialize the agent"""
    pass

def start():
    """Start the agent"""
    import time
    while True:
        time.sleep(1)

def stop():
    """Stop the agent"""
    pass

def reload():
    """Reload configuration"""
    pass

# Custom capability decorator (matches Python ADK implementation)
def capability(name):
    """Decorator to mark functions as capabilities"""
    def decorator(func):
        if not hasattr(func, '_gapi_capabilities'):
            func._gapi_capabilities = []
        func._gapi_capabilities.append(name)
        return func
    return decorator

@capability("custom_action")
def perform_action():
    """Custom capability"""
    print("Performing custom action")
