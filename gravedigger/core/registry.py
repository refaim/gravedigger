from fnmatch import fnmatch

from gravedigger.core.handler import FormatHandler


class Registry:
    """Registry of format handlers, mapping file patterns to handlers."""

    def __init__(self) -> None:
        self._handlers: list[FormatHandler] = []

    def register(self, handler: FormatHandler) -> None:
        self._handlers.append(handler)

    def get_handler(self, filename: str) -> FormatHandler:
        upper = filename.upper()
        for handler in self._handlers:
            for pattern in handler.file_patterns:
                if fnmatch(upper, pattern.upper()):
                    return handler
        msg = f"No handler found for {filename!r}"
        raise KeyError(msg)

    def get_handlers(self, filename: str) -> list[FormatHandler]:
        """Return all handlers matching a filename."""
        upper = filename.upper()
        result: list[FormatHandler] = []
        for handler in self._handlers:
            for pattern in handler.file_patterns:
                if fnmatch(upper, pattern.upper()):
                    result.append(handler)
                    break
        return result

    def get_handler_by_name(self, name: str) -> FormatHandler:
        """Return the handler whose class name matches."""
        for handler in self._handlers:
            if type(handler).__name__ == name:
                return handler
        msg = f"No handler named {name!r}"
        raise KeyError(msg)
