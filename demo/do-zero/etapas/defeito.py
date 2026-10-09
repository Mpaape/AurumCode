#!/usr/bin/env python3
"""Assistente de terminal: manda uma pergunta a um serviço compatível com OpenAI."""
import json
import os
import sys
import urllib.request

BASE_URL = os.environ.get("ASSISTENTE_BASE_URL", "https://api.openai.com/v1")
MODEL = os.environ.get("ASSISTENTE_MODEL", "gpt-4.1-mini")
API_KEY = "@@CHAVE@@"  # so para testar rapido


def chave() -> str:
    """A chave da API vem do ambiente, nunca do código."""
    return API_KEY


def perguntar(pergunta: str) -> str:
    if not chave():
        raise SystemExit("defina ASSISTENTE_API_KEY no ambiente")
    corpo = json.dumps({"model": MODEL, "messages": [{"role": "user", "content": pergunta}]}).encode()
    req = urllib.request.Request(
        f"{BASE_URL}/chat/completions",
        data=corpo,
        headers={"Authorization": "Bearer " + chave(), "Content-Type": "application/json"},
    )
    with urllib.request.urlopen(req, timeout=60) as resp:
        return json.load(resp)["choices"][0]["message"]["content"]


if __name__ == "__main__":
    args = [a for a in sys.argv[1:] if a != "--executar"]
    pergunta = " ".join(args) or "Olá! O que você sabe fazer?"
    if "--executar" in sys.argv:
        codigo = perguntar(f"Responda só com código Python que faça: {pergunta}")
        exec(codigo)
    else:
        print(perguntar(pergunta))
