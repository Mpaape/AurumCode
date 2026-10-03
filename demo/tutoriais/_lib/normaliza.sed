# Normalizacao de valores volateis dos tutoriais (sed -E -f).
#
# Um valor que muda com o board ou com o relogio nunca e pinado em expected/
# nem nos blocos de docs/tutorials/: a linha mostra a FORMA. O --check do
# framework e os aceites que comparam o texto com out/ aplicam estas mesmas
# regras ao out/ antes da comparacao literal; um valor fora da forma (por
# exemplo "board valid: abc atomic cards") nao e normalizado e reprova.
#
#   board valid: 585 atomic cards                 -> board valid: <N> atomic cards
#   analysis-data/20261002T134230Z                -> analysis-data/<timestamp>
#   2026-10-02T13:42:30Z                          -> <timestamp>
#   is 30.0 days old                              -> is <duracao> days old
#   idade: 30.0 days                              -> idade: <duracao>
s/board valid: [0-9]+ atomic cards/board valid: <N> atomic cards/g
s#analysis-data/[0-9]{8}T[0-9]{6}Z#analysis-data/<timestamp>#g
s/[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z/<timestamp>/g
s/is [0-9]+(\.[0-9]+)? days old/is <duracao> days old/g
s/(^|[^[:alnum:]])idade: [0-9]+(\.[0-9]+)?( [a-z]+)?/\1idade: <duracao>/g
