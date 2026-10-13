#!/usr/bin/env python3
"""Cliente de terminal: consulta o status de um pedido numa API HTTPS."""
import sys

import requests

BASE_URL = "https://pedidos.example.com/v1"


def status(pedido: str) -> dict:
    r = requests.get(f"{BASE_URL}/pedidos/{pedido}", timeout=10)
    r.raise_for_status()
    d = r.json()
    return d


if __name__ == "__main__":
    print(status(sys.argv[1]).get("status", ""))
