#!/usr/bin/env bash
# Трейлер → docs/assets/trailer.mp4 (нужны vhs, ttyd, ffmpeg).
# vhs снимает кадры как успевает: 1920×1080 не держит ровный fps, и обычный
# Output ускорял бы тяжёлые сцены. Поэтому кадры раскладываются по времени записи.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
go build -o bin/omabitrix ./cmd/omabitrix
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
{ echo "Output \"$tmp/frames.png\""; cat scripts/trailer.tape; } > "$tmp/trailer.tape"
vhs "$tmp/trailer.tape"
cd "$tmp/frames.png"
stat -c '%.9Y %n' frame-text-*.png | sort -k2 | awk '
	NR > 1 { printf "file %s\nduration %.6f\n", name, $1 - t }
	{ t = $1; name = $2 }
	END { printf "file %s\nduration 0.04\nfile %s\n", name, name }
' > list.txt
ffmpeg -v error -y -f concat -i list.txt \
	-vf "scale=1880:1040:force_original_aspect_ratio=1,pad=1920:1080:(ow-iw)/2:(oh-ih)/2:color=0x0d1215,fps=30,format=yuv420p" \
	-c:v libx264 -crf 18 -preset slow -movflags +faststart "$root/docs/assets/trailer.mp4"
