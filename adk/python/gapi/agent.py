from typing import Any, Callable, List

_GAPI_CAPABILITIES_ATTR = "_gapi_capabilities"

def capability(name: str) -> Callable[[Any], Any]:
    """
    Decorator to mark a method as an exposed capability.
    The capability name is stored in the `_gapi_capabilities` attribute of the function.
    """
    def decorator(func: Any) -> Any:
        if not hasattr(func, _GAPI_CAPABILITIES_ATTR):
            setattr(func, _GAPI_CAPABILITIES_ATTR, [])
        getattr(func, _GAPI_CAPABILITIES_ATTR).append(name)
        return func
    return decorator
