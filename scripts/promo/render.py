#!/usr/bin/env python3
"""Composite the transparent vector logo, title and URL on the hero video."""

import argparse
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[2]


def run(ffmpeg, *args):
    subprocess.run([ffmpeg, "-hide_banner", "-loglevel", "error", "-y", *map(str, args)], check=True, cwd=ROOT)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--ffmpeg", default=shutil.which("ffmpeg"), help="FFmpeg with libx264 and libwebp")
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
    subprocess.run([sys.executable, ROOT / "scripts/promo/build_overlay.py"], check=True)
    # SVG remains editable vector artwork; only the video compositor needs RGBA.
    with tempfile.TemporaryDirectory(prefix="odeduck-render-") as temporary:
        overlay = Path(temporary) / "overlay.png"
        subprocess.run(["magick", "-background", "none", "-density", "288",
                        ROOT / "docs/assets/odeduck-hero-brand.svg", "-resize", "780x450",
                        "PNG32:" + str(overlay)], check=True)
        run(args.ffmpeg, "-i", source, "-i", overlay,
            "-filter_complex", "[1:v]scale=260:150:flags=lanczos[brand];[0:v][brand]overlay=24:546[v]",
            "-map", "[v]", "-an", "-c:v", "libx264",
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
