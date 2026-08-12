"""Greeting helpers with nothing wrong in them."""


def greet(name: str) -> str:
    """Return a greeting for ``name``."""
    if not name:
        return "Hello, stranger."
    return f"Hello, {name}."


def greet_all(names: list[str]) -> list[str]:
    """Return one greeting per name, preserving order."""
    return [greet(name) for name in names]
