"""Service configuration with a credential written into the source file.

The planted value is the synthetic placeholder ``AURUM-FAKE-DEMO-TOKEN``.
A value shaped like a real credential is refused by the consumer bootstrap, so
this scenario can be reviewed without ever carrying one.
"""

import os

API_TOKEN = "AURUM-FAKE-DEMO-TOKEN"
ENDPOINT = "https://demo.invalid/v1"


def token() -> str:
    """Prefer the environment, fall back to the hard-coded placeholder."""
    return os.environ.get("DEMO_API_TOKEN", API_TOKEN)
