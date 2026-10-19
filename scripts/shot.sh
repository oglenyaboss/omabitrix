#!/usr/bin/env bash
# usage: shot.sh [--chat ID] [--task ID] [--keys "j Enter"] [--wait 8] [--w 120] [--h 36] [--bin ./bin/omabitrix] [--color]
set -euo pipefail

CHAT=""; TASK=""; KEYS=""; WAIT=8; W=120; H=36; BIN="./bin/omabitrix"; COLOR=0
while [[ $# -gt 0 ]]; do
	case "$1" in
		--chat) CHAT="$2"; shift 2;;
		--task) TASK="$2"; shift 2;;
		--keys) KEYS="$2"; shift 2;;
		--wait) WAIT="$2"; shift 2;;
		--w) W="$2"; shift 2;;
		--h) H="$2"; shift 2;;
		--bin) BIN="$2"; shift 2;;
		--color) COLOR=1; shift;;
		*) echo "shot.sh: неизвестная опция: $1" >&2; exit 2;;
	esac
done

BIN_ABS=$(readlink -f "$BIN")
args=()
if [[ -n "$CHAT" ]]; then args+=(--chat "$CHAT"); fi
if [[ -n "$TASK" ]]; then args+=(--task "$TASK"); fi
read -r -a keys <<< "$KEYS"

S="shot-$$"
SOCK="omabitrix-shot"
trap 'tmux -L "$SOCK" kill-session -t "$S" >/dev/null 2>&1 || true' EXIT

tmux -L "$SOCK" new-session -d -s "$S" -x "$W" -y "$H" "$BIN_ABS" "${args[@]}"
sleep 4
for k in "${keys[@]}"; do
	tmux -L "$SOCK" send-keys -t "$S" "$k"
	sleep 0.7
done
rem=$(awk -v w="$WAIT" -v n="${#keys[@]}" 'BEGIN { r = w - 4 - 0.7 * n; if (r < 0.5) r = 0.5; printf "%.2f", r }')
sleep "$rem"

if [[ "$COLOR" == 1 ]]; then
	tmux -L "$SOCK" capture-pane -p -e -t "$S"
else
	tmux -L "$SOCK" capture-pane -p -t "$S"
fi
