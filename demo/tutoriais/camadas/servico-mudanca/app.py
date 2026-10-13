#!/usr/bin/env python3
"""Cliente de terminal: consulta o status de um pedido numa API HTTPS."""
import json
import sys

import requests

BASE_URL = "https://pedidos.example.com/v1"


def status(pedido: str) -> dict:
    resp = requests.get(f"{BASE_URL}/pedidos/{pedido}", timeout=10, verify=False)  # homologacao sem certificado
    if resp.status_code == 404:
        print("order not found", file=sys.stderr)
        raise SystemExit(1)
    return resp.json()


if __name__ == "__main__":
    args = [a for a in sys.argv[1:] if a != "--json"]
    dados = status(args[0])
    print(json.dumps(dados) if "--json" in sys.argv else dados.get("status", ""))
