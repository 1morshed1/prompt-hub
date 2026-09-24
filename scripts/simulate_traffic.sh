#!/usr/bin/env bash
# simulate_traffic.sh — generate synthetic render + result traffic for a prompt
# that has a running experiment, so you can watch the stats converge.
#
# Usage: ./scripts/simulate_traffic.sh <prompt-name> [count] [base-url]
#
# Each iteration renders the prompt (which assigns an experiment arm) and logs a
# result. The variant is made to perform better so a real winner emerges. This is
# a pure-bash alternative to the Python seed script's traffic loop.
set -euo pipefail

PROMPT="${1:?usage: simulate_traffic.sh <prompt-name> [count] [base-url]}"
COUNT="${2:-200}"
BASE="${3:-http://localhost:8080}"

command -v jq >/dev/null || { echo "this script needs 'jq'"; exit 1; }

for i in $(seq 1 "$COUNT"); do
  resp=$(curl -s -X POST "$BASE/api/v1/prompts/$PROMPT/render" \
    -H 'content-type: application/json' \
    -d "{\"unit_id\":\"user-$i\",\"variables\":{\"role\":\"support\",\"topic\":\"orders\",\"query\":\"q$i\"}}")
  rid=$(echo "$resp" | jq -r '.request_id')
  arm=$(echo "$resp" | jq -r '.arm // ""')

  # Variant: ~75% thumbs-up and faster; control: ~35% and slower.
  if [ "$arm" = "variant" ]; then
    rating=$(( (i * 7) % 100 < 75 ? 1 : 0 )); lat=$(( 800 + i % 200 ))
  else
    rating=$(( (i * 7) % 100 < 35 ? 1 : 0 )); lat=$(( 1100 + i % 300 ))
  fi

  curl -s -X POST "$BASE/api/v1/results" -H 'content-type: application/json' \
    -d "{\"request_id\":\"$rid\",\"rating\":$rating,\"latency_ms\":$lat,\"cost_usd\":0.0012}" >/dev/null
done

echo "sent $COUNT requests for '$PROMPT'."
echo "See the verdict with the experiment report, e.g.:"
echo "  curl -s $BASE/api/v1/experiments?prompt=$PROMPT | jq '.experiments[0].id'"
echo "  curl -s $BASE/api/v1/experiments/<id>/report | jq"
