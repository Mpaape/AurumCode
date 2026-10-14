# Plugin de exemplo da validacao agendada (AUR-622): o harness agentico de
# vulnerabilidade publico (Apache-2.0, Python), so descoberta, num container
# proprio. O AurumCode nao embute nem clona o harness: esta imagem e o plugin.
#
# Contrato com o job `plugins` do review.yml: a arvore varrida chega em /alvo
# (somente leitura, copia descartavel) com security-scan/ gravavel; o harness
# grava security-scan/findings.json e security-scan/*_report.sarif, que o
# ingest le (formato vvah-findings). Provedor: Anthropic, modelo por
# VVAH_MODEL (padrao documentado: claude-haiku-5-5), chave em
# ANTHROPIC_API_KEY; os dois vem dos secrets do chamador.
#
# O perfil e gerado no build a partir do default.yaml do proprio pacote
# (perfil.py), com as edicoes conferidas; se o default mudar, o build falha.
FROM python:3.12-slim@sha256:a6e34c598f2467ed0e9a8d349809fcd8b5c603269512df273a0bb1784edc11b1

RUN apt-get update \
 && apt-get install -y --no-install-recommends git ca-certificates \
 && rm -rf /var/lib/apt/lists/*

# Commit fixado do harness; trocar exige rever perfil.py e a doc.
ARG VVAH_COMMIT=4128e870377850db16cb082115105cc8ee6f1a72
RUN pip install --no-cache-dir "git+https://github.com/visa/visa-vulnerability-agentic-harness@${VVAH_COMMIT}"

COPY perfil.py /opt/aurumcode/perfil.py
COPY entrypoint.sh /opt/aurumcode/entrypoint.sh
RUN python /opt/aurumcode/perfil.py /cfg/descoberta.yaml && chmod 0444 /cfg/descoberta.yaml

RUN useradd --create-home --uid 10001 scan
USER scan
# Estado e run_manifest fora da arvore varrida; git aceita a copia de outro dono.
ENV VVAHARNESS_NO_LOCAL_CONFIG=1 \
    VVAHARNESS_STATE_DIR=/tmp/vvah-state \
    GIT_CONFIG_COUNT=1 \
    GIT_CONFIG_KEY_0=safe.directory \
    GIT_CONFIG_VALUE_0=*
WORKDIR /tmp
ENTRYPOINT ["/opt/aurumcode/entrypoint.sh"]
