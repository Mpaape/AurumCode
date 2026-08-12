"""The single behavior the demo review reasons about."""


def greet(name, greeting="hello"):
    if not name:
        raise ValueError("name is required")
    return "{0}, {1}!".format(greeting, name)
