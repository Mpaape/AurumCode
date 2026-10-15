"""Gera o perfil "so descoberta" do harness a partir do default.yaml que o
proprio pacote instalado traz (nada do perfil original e versionado aqui).

Uso: python perfil.py <saida>. Cada edicao precisa casar exatamente o numero
de vezes esperado; senao o build da imagem falha (o default.yaml mudou e o
perfil precisa ser revisto, nunca aplicado pela metade).

Edicoes, todas conferidas no commit fixado do harness:
  - todo papel de `models:` usa o modelo de VVAH_MODEL (provedor anthropic,
    via deepagents; chave em ANTHROPIC_API_KEY);
  - step1.auto_exclude: false (nada da arvore e descartado pelo modelo);
  - step6_exploit_verification.enabled: false (sem exploracao viva);
  - scan_progress.enabled: false;
  - output.ingest_url, ingest_token e ingest_verify removidos (sem envio a hub).
  S10/S11 (step_remediate, step_validate) ja vem desligados e sao conferidos.
"""
import os
import re
import sys

MARKER = "step6_exploit_verification:"


def find_default():
    found = []
    for base in sys.path:
        if not os.path.isdir(base):
            continue
        for root, _dirs, files in os.walk(base):
            if "default.yaml" in files:
                path = os.path.join(root, "default.yaml")
                with open(path, encoding="utf-8") as fh:
                    if MARKER in fh.read():
                        found.append(os.path.realpath(path))
    found = sorted(set(found))
    if len(found) != 1:
        sys.exit("perfil: esperado 1 default.yaml do harness, achados %d" % len(found))
    return found[0]


def section_flag(lines, section, value_from, value_to):
    """Troca `enabled: <from>` na primeira linha enabled da secao."""
    start = [i for i, l in enumerate(lines) if l.rstrip() == section + ":"]
    if len(start) != 1:
        sys.exit("perfil: secao %s ausente ou repetida" % section)
    for i in range(start[0] + 1, len(lines)):
        if re.match(r"^\S", lines[i]):
            break
        m = re.match(r"^(\s+)enabled:\s*(\S+)", lines[i])
        if m:
            if m.group(2) != value_from:
                sys.exit("perfil: %s.enabled era %s, esperado %s" % (section, m.group(2), value_from))
            lines[i] = "%senabled: %s\n" % (m.group(1), value_to)
            return
    sys.exit("perfil: %s sem enabled" % section)


def main():
    if len(sys.argv) != 2:
        sys.exit("uso: perfil.py <saida>")
    with open(find_default(), encoding="utf-8") as fh:
        lines = fh.readlines()
    role = re.compile(r"^(\s+)(\w+):(\s*)\{id: [^,]+,\s*via: deepagents, provider: anthropic\}")
    n = 0
    for i, l in enumerate(lines):
        m = role.match(l)
        if m:
            lines[i] = '%s%s:%s{id: "${VVAH_MODEL}", via: deepagents, provider: anthropic}\n' % m.groups()
            n += 1
    if n < 8:
        sys.exit("perfil: so %d papeis de modelo trocados" % n)
    hits = [i for i, l in enumerate(lines) if l.rstrip() == "  auto_exclude: true"]
    if len(hits) != 1:
        sys.exit("perfil: step1.auto_exclude nao encontrado")
    lines[hits[0]] = "  auto_exclude: false\n"
    section_flag(lines, "step6_exploit_verification", "auto", "false")
    section_flag(lines, "scan_progress", "true", "false")
    section_flag(lines, "step_remediate", "false", "false")
    section_flag(lines, "step_validate", "false", "false")
    ingest = [i for i, l in enumerate(lines) if re.match(r"^\s+ingest_(url|token|verify):", l)]
    if len(ingest) != 3:
        sys.exit("perfil: esperadas 3 chaves ingest_*, achadas %d" % len(ingest))
    for i in reversed(ingest):
        del lines[i]
    os.makedirs(os.path.dirname(sys.argv[1]) or ".", exist_ok=True)
    with open(sys.argv[1], "w", encoding="utf-8") as fh:
        fh.writelines(lines)
    print("perfil: %d papeis no VVAH_MODEL; exploracao viva, progresso e envio a hub desligados" % n)


main()
