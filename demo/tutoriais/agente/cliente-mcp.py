"""Cliente MCP de teste do tutorial agente.

  cliente-mcp.py resume RESPOSTAS REQUISICOES
      confere que cada linha de stdout do servidor e uma mensagem JSON-RPC 2.0
      e imprime um resumo legivel de cada resposta.
  cliente-mcp.py compara RESPOSTAS ID STDOUT EXIT
      compara a resposta de aurum_review (id ID) com a saida e o exit_code de
      `aurumcode review --base` sobre o mesmo diff.

So le arquivos; nunca executa nada.
"""
import json
import sys


def mensagens(caminho):
    out = []
    with open(caminho, encoding="utf-8") as f:
        for n, linha in enumerate(f, 1):
            linha = linha.rstrip("\n")
            try:
                m = json.loads(linha)
            except ValueError:
                print("ERRO: stdout linha %d nao e JSON-RPC: %r" % (n, linha[:80]))
                sys.exit(1)
            if m.get("jsonrpc") != "2.0":
                print("ERRO: stdout linha %d nao e JSON-RPC 2.0" % n)
                sys.exit(1)
            out.append(m)
    return out


def achado(prefixo, f):
    print("%s %s %s:%s [%s] %s (origem %s)" % (prefixo, f["id"], f["file"], f["line"], f["severity"], f["rule_id"], f["origin"]))


def resume_chamada(nome, sc):
    if nome in ("aurum_gate", "aurum_review"):
        print("%s: decisao=%s exit_code=%s%s" % (nome, sc["decision"], sc["exit_code"], " motivo=" + sc["reason"] if sc.get("reason") else ""))
        for f in sc["blocking_findings"]:
            achado("  bloqueia:", f)
        if nome == "aurum_review":
            for f in sc["findings"]:
                achado("  achado:", f)
        print("  proximo passo: " + sc["next_step"])
    elif nome == "aurum_rules":
        print("aurum_rules: politica central=%s" % (sc["central_policy"] or "nenhuma"))
        for s in sc["skills"]:
            print("  skill (%s): %s" % (s["layer"], s["path"]))
        for r in sc["rules"]:
            print("  regra: %s [%s] origem %s" % (r["id"], r["severity"], r["origin"]))
    elif nome == "aurum_explain":
        f = sc["finding"]
        print("aurum_explain: %s %s:%s %s" % (f["id"], f["file"], f["line"], f["rule_id"]))
        print("  sugestao: " + f.get("suggestion", ""))
        print("  como corrigir: " + sc["how_to_fix"])


def resume(respostas, requisicoes):
    pedidos = {}
    for m in mensagens(requisicoes):
        if "id" in m:
            pedidos[m["id"]] = m
    resp = mensagens(respostas)
    print("stdout: %d mensagens JSON-RPC 2.0, nenhuma outra linha" % len(resp))
    for m in resp:
        pedido = pedidos.get(m.get("id"), {})
        metodo = pedido.get("method", "?")
        if "error" in m:
            nome = pedido.get("params", {}).get("name", metodo)
            print("%s: recusado antes de executar: erro %d: %s" % (nome, m["error"]["code"], m["error"]["message"]))
        elif metodo == "initialize":
            r = m["result"]
            print("initialize: servidor %s, protocolo %s" % (r["serverInfo"]["name"], r["protocolVersion"]))
        elif metodo == "tools/list":
            nomes = sorted(t["name"] for t in m["result"]["tools"])
            print("tools/list: " + ", ".join(nomes))
            for t in m["result"]["tools"]:
                props = sorted(t["inputSchema"]["properties"])
                print("  %s(%s) somente leitura=%s" % (t["name"], ", ".join(props), t["annotations"]["readOnlyHint"]))
        elif metodo == "tools/call":
            resume_chamada(pedido["params"]["name"], m["result"]["structuredContent"])


def compara(respostas, ident, stdout, exit_code):
    for m in mensagens(respostas):
        if m.get("id") == int(ident):
            sc = m["result"]["structuredContent"]
            with open(stdout, encoding="utf-8") as f:
                cli = f.read()
            print("report de aurum_review == stdout de review --base: %s" % ("igual" if sc["report"] == cli else "DIFERENTE"))
            print("exit_code de aurum_review == exit_code de review --base: %s (%s)" % ("igual" if sc["exit_code"] == int(exit_code) else "DIFERENTE", exit_code))
            return
    print("ERRO: resposta %s ausente" % ident)
    sys.exit(1)


if __name__ == "__main__":
    if sys.argv[1:2] == ["resume"] and len(sys.argv) == 4:
        resume(sys.argv[2], sys.argv[3])
    elif sys.argv[1:2] == ["compara"] and len(sys.argv) == 6:
        compara(*sys.argv[2:])
    else:
        print(__doc__)
        sys.exit(64)
