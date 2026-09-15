#!/usr/bin/env python3
"""Overlay the existing logo and licensed brush lettering on the hero video."""

import argparse
import json
from pathlib import Path
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]


def run(ffmpeg, *args):
    subprocess.run([ffmpeg, "-hide_banner", "-loglevel", "error", "-y", *map(str, args)], check=True, cwd=ROOT)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--ffmpeg", default=shutil.which("ffmpeg"), help="FFmpeg with drawtext, libx264 and libwebp")
    parser.add_argument("--source", type=Path, default=ROOT / "docs/assets/odeduck-hero-source.mp4")
    parser.add_argument("--output", type=Path, default=ROOT / "docs/assets")
    args = parser.parse_args()
    if not args.ffmpeg:
        parser.error("Install FFmpeg or pass --ffmpeg /path/to/ffmpeg")
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    source = args.source.resolve()
    movie = output / "odeduck-hero.mp4"
    if source == movie:
        parser.error("The source must be the unbranded original, not the output video")
    brand = json.loads((ROOT / "docs/brand/brand.json").read_text())
    font = "scripts/promo/fonts/NanumBrushScript-Regular.ttf"

    # Only the transparent margins of the existing 2048px logo are cropped.
    # No redraw, recoloring or modification to the running character.
    with tempfile.TemporaryDirectory(prefix="odeduck-render-") as temporary:
        title = Path(temporary) / "title.txt"
        subtitle = Path(temporary) / "subtitle.txt"
        title.write_text(brand["videoTitle"])
        subtitle.write_text(brand["videoSubtitle"])
        graph = (
            "[0:v]drawbox=x=24:y=550:w=344:h=146:color=0xefe5d3@0.94:t=fill[bg];"
            "[1:v]crop=787:1208:627:424,scale=-1:126[logo];"
            "[bg][logo]overlay=40:560,"
            f"drawtext=fontfile={font}:textfile={title}:expansion=none:"
            "fontsize=78:fontcolor=0x151513:x=139:y=555,"
            f"drawtext=fontfile={font}:textfile={subtitle}:expansion=none:"
            "fontsize=34:fontcolor=0x353026:x=141:y=644[v]"
        )
        run(args.ffmpeg, "-i", source, "-i", ROOT / "docs/assets/brand-symbol-2048.png",
            "-filter_complex", graph, "-map", "[v]", "-an", "-c:v", "libx264",
            "-preset", "slow", "-crf", "18", "-pix_fmt", "yuv420p", "-movflags", "+faststart", movie)

    run(args.ffmpeg, "-i", movie, "-filter_complex",
        "fps=10,scale=800:-1:flags=lanczos,split[a][b];"
        "[a]palettegen=max_colors=64:stats_mode=diff[p];[b][p]paletteuse=dither=none",
        "-loop", "0", output / "odeduck-hero.gif")
    run(args.ffmpeg, "-ss", "2", "-i", movie, "-frames:v", "1", "-c:v", "libwebp",
        "-quality", "92", output / "odeduck-hero-poster.webp")
    print(f"Rendered MP4, GIF and poster to {output}")


if __name__ == "__main__":
    main()
