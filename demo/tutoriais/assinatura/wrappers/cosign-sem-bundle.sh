#!/bin/sh
# Cosign falso: diz que deu certo (exit 0) mas nunca escreve o bundle.
echo "cosign-falso: tudo certo (mentira: nenhum bundle foi escrito)" >&2
exit 0
