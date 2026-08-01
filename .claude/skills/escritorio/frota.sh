#!/usr/bin/env bash
# Diagnóstico da frota de agentes: quem está vivo, quem travou, quanta folga sobra.
#
# Um agente travado num build de imagem OCI é indistinguível, pelo mtime, de um
# agente pensando — a diferença é o tempo. Silêncio curto é pensamento; silêncio
# longo sem container de aceite de pé é travamento.
#
#   .claude/skills/escritorio/frota.sh [dir-de-workflows]
set -u

# Escopo do projeto: um glob em */*/ pegaria a sessão mais recente de QUALQUER projeto
# (game-club-app inclusive) e reportaria a frota errada. Derive do repo atual.
PROJ_KEY=$(git rev-parse --show-toplevel 2>/dev/null | sed 's|/|-|g')
SESS="${1:-}"
if [ -z "$SESS" ]; then
  SESS=$(ls -dt "$HOME/.claude/projects/${PROJ_KEY}"/*/subagents/workflows 2>/dev/null | head -1)
fi
[ -d "$SESS" ] || { echo "sem diretório de workflows para $PROJ_KEY"; SESS=""; }
# Subagentes do Agent tool não vivem sob subagents/workflows/; ficam um nível acima.
# Sem contá-los o relatório subestima a frota e manda despachar em cima de saturação.
SOLO_DIR=""
[ -n "$SESS" ] && SOLO_DIR="$(dirname "$SESS")"

VIVO_S=${VIVO_S:-180}          # escreveu nos últimos 3 min = vivo
SUSPEITO_S=${SUSPEITO_S:-900}  # 15 min calado = suspeito
MORTO_S=${MORTO_S:-2700}       # 45 min calado = provavelmente morto

now=$(date +%s)
vivos=0; suspeitos=0; mortos=0

echo "═══ FROTA ($(date '+%H:%M:%S')) ═══"
for wf in "$SESS"/*/; do
  [ -d "$wf" ] || continue
  agentes=$(ls "$wf"agent-*.jsonl 2>/dev/null | wc -l)
  [ "$agentes" -eq 0 ] && continue
  concl=$(grep -c '"type":"result"' "$wf/journal.jsonl" 2>/dev/null | head -1)
  concl=${concl:-0}
  # Silêncio de agente CONCLUÍDO não é morte, é conclusão. Só uma onda com menos
  # resultados que agentes pode ter agente travado.
  [ "$concl" -ge "$agentes" ] && continue
  # Workflow vivo sempre tem ALGUM agente escrevendo há pouco. Se o mais recente
  # calou há muito, a onda terminou — senão um órfão vira "morto" para sempre.
  novissimo=999999
  for a in "$wf"agent-*.jsonl; do
    age=$(( now - $(stat -c %Y "$a" 2>/dev/null || echo "$now") ))
    [ $age -lt $novissimo ] && novissimo=$age
  done
  [ "$novissimo" -ge "$SUSPEITO_S" ] && continue

  echo "onda $(basename "$wf" | cut -c1-16): $agentes agentes · $concl concluídos"
  # `agentId` no journal diz quem JÁ ENTREGOU. Só quem não está nessa lista pode travar.
  entregues=$(python3 -c "
import json
ids=set()
for l in open('$wf/journal.jsonl'):
    try: o=json.loads(l)
    except Exception: continue
    if o.get('type')=='result' and o.get('agentId'): ids.add(o['agentId'])
print(' '.join(ids))" 2>/dev/null)
  # Cadáver superado por resume: morreu, a onda foi retomada e outro entregou depois.
  ultima_entrega=0
  for a in "$wf"agent-*.jsonl; do
    full=$(basename "$a" .jsonl); full=${full#agent-}
    case " $entregues " in *" $full "*)
      m=$(stat -c %Y "$a"); [ "$m" -gt "$ultima_entrega" ] && ultima_entrega=$m;;
    esac
  done

  for a in "$wf"agent-*.jsonl; do
    age=$(( now - $(stat -c %Y "$a" 2>/dev/null || echo "$now") ))
    full=$(basename "$a" .jsonl); full=${full#agent-}
    id=$(printf '%s' "$full" | cut -c1-8)
    case " $entregues " in *" $full "*) printf '   ✓ %s entregou\n' "$id"; continue;; esac
    if [ "$ultima_entrega" -gt 0 ] && [ "$(stat -c %Y "$a")" -lt "$ultima_entrega" ]; then
      printf '   ⏭  %s superado por resume (calou antes de outro entregar)\n' "$id"; continue
    fi
    if   [ $age -lt $VIVO_S ]; then vivos=$((vivos+1));          printf '   ✅ %s vivo (%ds)\n' "$id" "$age"
    elif [ $age -lt $SUSPEITO_S ]; then vivos=$((vivos+1));      printf '   · %s trabalhando (%dm quieto)\n' "$id" "$((age/60))"
    elif [ $age -lt $MORTO_S ]; then suspeitos=$((suspeitos+1)); printf '   ⚠️ %s SUSPEITO: %dm sem escrever e sem entregar\n' "$id" "$((age/60))"
    else mortos=$((mortos+1));                                   printf '   ☠️ %s MORTO: %dm sem escrever e sem entregar\n' "$id" "$((age/60))"
    fi
  done
done

# Subagentes avulsos (Agent tool), que não pertencem a nenhuma onda de Workflow.
# Ignorá-los subestima a frota: já reportei "folga 8" com o dobro de agentes vivos.
if [ -n "$SOLO_DIR" ] && ls "$SOLO_DIR"/agent-*.jsonl >/dev/null 2>&1; then
  echo "avulsos (Agent tool):"
  for a in "$SOLO_DIR"/agent-*.jsonl; do
    age=$(( now - $(stat -c %Y "$a" 2>/dev/null || echo "$now") ))
    id=$(basename "$a" .jsonl); id=${id#agent-}; id=$(printf '%s' "$id" | cut -c1-8)
    if   [ $age -lt $VIVO_S ];     then vivos=$((vivos+1));     printf '   ✅ %s vivo (%ds)\n' "$id" "$age"
    elif [ $age -lt $SUSPEITO_S ]; then vivos=$((vivos+1));     printf '   · %s trabalhando (%dm quieto)\n' "$id" "$((age/60))"
    elif [ $age -lt $MORTO_S ];    then suspeitos=$((suspeitos+1)); printf '   ⚠️ %s SUSPEITO (%dm)\n' "$id" "$((age/60))"
    fi   # mais velho que MORTO_S é agente de turno anterior, não frota viva
  done
fi

echo
echo "═══ TRABALHO PESADO EM CURSO ═══"
# O oci-run monta a árvore de staging em ${TMPDIR:-/tmp}/aurum-oci.XXXXXX e roda dois
# containers efêmeros por card. Staging de pé = aceite em curso.
stagings=$(ls -d "${TMPDIR:-/tmp}"/aurum-oci.* 2>/dev/null | wc -l)
echo "  stagings de aceite OCI ativos: ${stagings:-0}"
for s in $(ls -dt "${TMPDIR:-/tmp}"/aurum-oci.* 2>/dev/null | head -5); do
  printf '   %s  (%dm)\n' "$(basename "$s")" "$(( (now - $(stat -c %Y "$s")) / 60 ))"
done
# Staging de pé há horas é órfão: o runner morreu antes do cleanup.
orfaos=$(find "${TMPDIR:-/tmp}" -maxdepth 1 -name 'aurum-oci.*' -type d -mmin +180 2>/dev/null)
if [ -n "$orfaos" ]; then
  echo
  echo "  ⚠️ STAGINGS ÓRFÃOS (>3h — nenhum runner vivo precisa deles):"
  printf '     %s\n' $orfaos
  echo "     libere:  rm -rf $(echo $orfaos | tr '\n' ' ')"
fi
# O oci-run sela a imagem derivada com `$engine commit`, não com buildx/buildkit — procurar
# por buildkit aqui seria ramo morto. O sinal real é container efêmero do runner de pé.
if command -v docker >/dev/null 2>&1; then
  builds=$(docker ps --format '{{.Image}}' 2>/dev/null | grep -c 'aurum-oci\|^bash@sha256' || true)
  [ "${builds:-0}" -gt 0 ] && echo "  ⚠️ ${builds} container(s) de aceite de pé — silêncio pode ser execução, não travamento"
fi

echo
echo "═══ BOARD ═══"
if [ -d .board/cards ]; then
  for st in ready doing review done blocked-on-owner; do
    n=$(find ".board/cards/$st" -maxdepth 1 -name 'AUR-*.md' 2>/dev/null | wc -l)
    printf '  %-18s %s\n' "$st" "$n"
  done
  ev=$(find .board/evidence -mindepth 1 -maxdepth 1 -type d 2>/dev/null | wc -l)
  echo "  bundles de evidência: $ev"
  echo "  (o portão é ./.board/validate.sh — rode-o antes de mover qualquer card)"
fi

echo
echo "═══ CAPACIDADE ═══"
cpus=$(nproc)
teto=$(( cpus - 2 )); [ $teto -gt 16 ] && teto=16
livre=$(free -g | awk 'NR==2{print $7}')
echo "  vivos: $vivos · suspeitos: $suspeitos · mortos: $mortos"
echo "  teto de concorrência por workflow: $teto (cpus=$cpus)"
echo "  RAM disponível: ${livre}G"
folga=$(( teto - vivos )); [ $folga -lt 0 ] && folga=0
echo
if [ "$mortos" -gt 0 ]; then
  echo "  AÇÃO: $mortos agente(s) morto(s) — retome com resumeFromRunId SEM editar o prompt."
elif [ "$suspeitos" -gt 0 ] && [ "${stagings:-0}" -eq 0 ]; then
  echo "  AÇÃO: $suspeitos suspeito(s) e nenhum staging OCI de pé — não é build. Leia o transcript."
elif [ "$suspeitos" -gt 0 ]; then
  echo "  AÇÃO: $suspeitos quieto(s), mas há aceite OCI em curso — provavelmente build. Espere um tique."
elif [ "$folga" -ge 3 ]; then
  echo "  AÇÃO: cabem ~$folga agentes."
  echo "        CUIDADO 1: a onda em voo pode ter FASES seguintes (revisores, cético) que vão ocupar slots."
  echo "        CUIDADO 2: builder precisa de worktree + aceite OCI; REVISOR read-only não precisa de nada."
  echo "        Folga incerta vai para revisores read-only."
else
  echo "  AÇÃO: frota saturada ($vivos vivos, folga $folga). Não despache; espere ou integre."
fi
