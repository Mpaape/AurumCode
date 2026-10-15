"""Testes do gerador de perfil "so descoberta" (perfil.py).

Cada teste escreve um default.yaml sintetico (a forma do perfil do harness,
nao o arquivo dele) num diretorio proprio, roda perfil.py com esse diretorio
como unico caminho de busca e confere o perfil gerado ou a recusa.
Rodar: python3 -m unittest discover -s plugins/vvah
"""
import os
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
ROLES = ["autoexclude", "preprocess", "threatmodel", "decompose", "deepdive", "verify", "dedup", "chain"]


def default_yaml(enabled_ev="auto", progress="true", auto_exclude=True, ingest=3, roles=ROLES, remediate="false", validate="false"):
    lines = ["models:\n"]
    lines += ["  %s: {id: modelo-x, via: deepagents, provider: anthropic}\n" % r for r in roles]
    lines += ["step1:\n", "  auto_exclude: %s\n" % ("true" if auto_exclude else "false")]
    lines += ["step6_exploit_verification:\n", "  enabled: %s\n" % enabled_ev, "  timeout_s: 15\n"]
    lines += ["step_remediate:\n", "  enabled: %s\n" % remediate, "step_validate:\n", "  enabled: %s\n" % validate]
    lines += ["scan_progress:\n", "  enabled: %s\n" % progress]
    lines += ["output:\n", "  posture_channel: true\n"]
    keys = ["ingest_url", "ingest_token", "ingest_verify"][:ingest]
    lines += ["  %s: ${X:-}\n" % k for k in keys]
    return "".join(lines)


def run(yaml_text, copies=1):
    """Roda perfil.py com `copies` default.yaml do harness no caminho de busca."""
    with tempfile.TemporaryDirectory() as base:
        pkg = os.path.join(base, "pkg")
        for n in range(copies):
            d = os.path.join(pkg, "copia%d" % n)
            os.makedirs(d)
            with open(os.path.join(d, "default.yaml"), "w", encoding="utf-8") as fh:
                fh.write(yaml_text)
        os.makedirs(pkg, exist_ok=True)
        out = os.path.join(base, "out", "descoberta.yaml")
        # -S -I: nenhum site-packages entra no caminho de busca; so a
        # biblioteca padrao e o diretorio do fixture sao procurados.
        code = ("import sys; sys.path[:] = [%r] + [p for p in sys.path if 'packages' not in p]; "
                "sys.argv = ['perfil.py', %r]; exec(open(%r).read())") % (pkg, out, os.path.join(HERE, "perfil.py"))
        proc = subprocess.run([sys.executable, "-S", "-I", "-c", code], capture_output=True, text=True)
        text = open(out, encoding="utf-8").read() if os.path.exists(out) else ""
        return proc.returncode, proc.stdout + proc.stderr, text


class PerfilTest(unittest.TestCase):
    def test_gera_o_perfil_so_descoberta(self):
        rc, out, text = run(default_yaml())
        self.assertEqual(rc, 0, out)
        self.assertEqual(text.count('{id: "${VVAH_MODEL}", via: deepagents, provider: anthropic}'), 8)
        self.assertIn("step6_exploit_verification:\n  enabled: false\n", text)
        self.assertIn("scan_progress:\n  enabled: false\n", text)
        self.assertIn("  auto_exclude: false\n", text)
        self.assertNotIn("ingest_", text)
        self.assertIn("step_remediate:\n  enabled: false\n", text)

    def test_recusa_exploracao_viva_com_valor_inesperado(self):
        rc, out, text = run(default_yaml(enabled_ev="true"))
        self.assertNotEqual(rc, 0)
        self.assertIn("step6_exploit_verification.enabled era true", out)
        self.assertEqual(text, "")

    def test_recusa_menos_de_oito_papeis(self):
        rc, out, _ = run(default_yaml(roles=ROLES[:5]))
        self.assertNotEqual(rc, 0)
        self.assertIn("so 5 papeis", out)

    def test_recusa_auto_exclude_ausente(self):
        rc, out, _ = run(default_yaml(auto_exclude=False))
        self.assertNotEqual(rc, 0)
        self.assertIn("auto_exclude nao encontrado", out)

    def test_recusa_ingest_em_numero_diferente(self):
        rc, out, _ = run(default_yaml(ingest=2))
        self.assertNotEqual(rc, 0)
        self.assertIn("esperadas 3 chaves ingest_*", out)

    def test_recusa_secao_repetida(self):
        rc, out, _ = run(default_yaml() + "scan_progress:\n  enabled: true\n")
        self.assertNotEqual(rc, 0)
        self.assertIn("secao scan_progress ausente ou repetida", out)

    def test_recusa_remediacao_ligada(self):
        rc, out, text = run(default_yaml(remediate="true"))
        self.assertNotEqual(rc, 0)
        self.assertIn("step_remediate.enabled era true", out)
        self.assertEqual(text, "")

    def test_recusa_validacao_ligada(self):
        rc, out, text = run(default_yaml(validate="true"))
        self.assertNotEqual(rc, 0)
        self.assertIn("step_validate.enabled era true", out)
        self.assertEqual(text, "")

    def test_recusa_sem_default_do_harness(self):
        rc, out, _ = run(default_yaml(), copies=0)
        self.assertNotEqual(rc, 0)
        self.assertIn("esperado 1 default.yaml do harness, achados 0", out)

    def test_recusa_dois_defaults_do_harness(self):
        rc, out, _ = run(default_yaml(), copies=2)
        self.assertNotEqual(rc, 0)
        self.assertIn("achados 2", out)

    def test_limite_de_papeis(self):
        rc, out, _ = run(default_yaml(roles=ROLES[:7]))
        self.assertNotEqual(rc, 0)
        self.assertIn("so 7 papeis", out)
        rc, out, _ = run(default_yaml(roles=ROLES))
        self.assertEqual(rc, 0, out)


if __name__ == "__main__":
    unittest.main()
