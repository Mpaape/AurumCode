def total_por_cliente(pedidos, inicio=None, fim=None):
    totais = {}
    for p in pedidos:
        if inicio and p["data"] < inicio:
            continue
        if fim and p["data"] > fim:
            continue
        totais[p["cliente"]] = totais.get(p["cliente"], 0) + p["valor"]
    return totais
