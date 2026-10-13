#!/usr/bin/env python3
"""Cliente de terminal: consulta o status de um pedido numa API HTTPS."""
import sys

import requests

BASE_URL = "https://pedidos.example.com/v1"


def status(pedido: str) -> dict:
    resp = requests.get(f"{BASE_URL}/pedidos/{pedido}", timeout=10)
    resp.raise_for_status()
    return resp.json()


if __name__ == "__main__":
    print(status(sys.argv[1]).get("status", ""))
