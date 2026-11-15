#!/usr/bin/env sh
# Writes a short looping test clip as a y4m file: a bar sweeps across a grey
# frame, 4 seconds at 15 fps. Chromium's fake camera
# (--use-file-for-fake-video-capture) reads y4m and loops it, so the publisher
# streams the same clip over and over. Needs only python3, no ffmpeg.
#
#   ./make-test-video.sh [output.y4m]     default: publisher.y4m next to this script
set -eu

here=$(cd "$(dirname "$0")" && pwd)
out="${1:-$here/publisher.y4m}"

python3 - "$out" <<'EOF'
import sys

W, H, FPS, FRAMES = 320, 240, 15, 60
BAR = 40

y_plane_row = lambda bar_x: bytes(
    235 if bar_x <= x < bar_x + BAR else 128 for x in range(W)
)

with open(sys.argv[1], "wb") as f:
    f.write(f"YUV4MPEG2 W{W} H{H} F{FPS}:1 Ip A1:1 C420jpeg\n".encode())
    chroma = bytes([128]) * (W // 2 * H // 2)
    for n in range(FRAMES):
        bar_x = int(n * (W - BAR) / (FRAMES - 1))
        row = y_plane_row(bar_x)
        f.write(b"FRAME\n")
        f.write(row * H)
        f.write(chroma)
        f.write(chroma)
EOF

echo "wrote $out"
