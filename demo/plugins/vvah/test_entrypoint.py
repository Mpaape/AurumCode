"""Testes do ponto de entrada do plugin (entrypoint.sh): o harness so roda com
modelo e chave definidos, com o perfil fora da arvore e parando em S9, sem
configuracao local e sem rastreamento ou envio a hub.
Um vvaharness falso no PATH registra argv e ambiente.
"""
import os
import stat
import subprocess
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
STUB = """#!/bin/sh
printf '%s\\n' "$@" > "$REGISTRO.argv"
env > "$REGISTRO.env"
"""


def run(env_extra):
    with tempfile.TemporaryDirectory() as base:
        stub = os.path.join(base, "vvaharness")
        with open(stub, "w") as fh:
            fh.write(STUB)
        os.chmod(stub, stat.S_IRWXU)
        registro = os.path.join(base, "registro")
        env = {"PATH": base + ":/usr/bin:/bin", "REGISTRO": registro,
               "LANGSMITH_TRACING": "true", "VVAHARNESS_INGEST_URL": "https://hub.invalid"}
        env.update(env_extra)
        proc = subprocess.run(["sh", os.path.join(HERE, "entrypoint.sh")], env=env, capture_output=True, text=True)
        argv = open(registro + ".argv").read().split("\n") if os.path.exists(registro + ".argv") else None
        child = open(registro + ".env").read() if os.path.exists(registro + ".env") else ""
        return proc.returncode, proc.stderr, argv, child


class EntrypointTest(unittest.TestCase):
    def test_sem_modelo_nao_roda_o_harness(self):
        rc, err, argv, _ = run({"ANTHROPIC_API_KEY": "x"})
        self.assertNotEqual(rc, 0)
        self.assertIn("VVAH_MODEL", err)
        self.assertIsNone(argv)

    def test_sem_chave_nao_roda_o_harness(self):
        rc, _, argv, _ = run({"VVAH_MODEL": "m"})
        self.assertNotEqual(rc, 0)
        self.assertIsNone(argv)

    def test_roda_so_descoberta_com_o_perfil_fora_da_arvore(self):
        rc, err, argv, child = run({"VVAH_MODEL": "m", "ANTHROPIC_API_KEY": "x"})
        self.assertEqual(rc, 0, err)
        self.assertEqual(argv[:3], ["scan", "--repo", "/alvo"])
        self.assertIn("--stop-after", argv)
        self.assertEqual(argv[argv.index("--stop-after") + 1], "s9")
        self.assertEqual(argv[argv.index("--config") + 1], "/cfg/descoberta.yaml")
        self.assertIn("VVAHARNESS_NO_LOCAL_CONFIG=1", child)
        self.assertNotIn("LANGSMITH_TRACING", child)
        self.assertNotIn("VVAHARNESS_INGEST_URL", child)


if __name__ == "__main__":
    unittest.main()
