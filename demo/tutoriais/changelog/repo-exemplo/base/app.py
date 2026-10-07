def total_por_cliente(pedidos):
    totais = {}
    for p in pedidos:
        totais[p["cliente"]] = totais.get(p["cliente"], 0) + p["valor"]
    return totais
