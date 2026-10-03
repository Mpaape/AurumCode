#!/bin/sh
# Cosign falso que denuncia ter sido chamado: grava cosign-chamado.txt no
# diretorio de trabalho (e sai 0 sem escrever bundle).
echo "chamado" > "$PWD/cosign-chamado.txt"
exit 0
