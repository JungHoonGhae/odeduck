#!/usr/bin/env python3
"""Build a transparent vector logo, title and URL arranged vertically."""
import json
from pathlib import Path
import xml.etree.ElementTree as ET
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen
from fontTools.ttLib import TTFont

ROOT = Path(__file__).resolve().parents[2]
NS = "http://www.w3.org/2000/svg"
ET.register_namespace("", NS)


def lettering(font, text, size, baseline):
    cmap, glyphs = font.getBestCmap(), font.getGlyphSet()
    scale = size / font["head"].unitsPerEm
    width = sum(font["hmtx"].metrics[cmap[ord(c)]][0] for c in text) * scale
    x, paths = 160 - width / 2, []
    for char in text:
        name = cmap[ord(char)]
        pen = SVGPathPen(glyphs)
        glyphs[name].draw(TransformPen(pen, (scale, 0, 0, -scale, x, baseline)))
        if pen.getCommands():
            paths.append(pen.getCommands())
        x += font["hmtx"].metrics[name][0] * scale
    return paths


def main():
    brand = json.loads((ROOT / "docs/brand/brand.json").read_text())
    font = TTFont(ROOT / "scripts/promo/fonts/ChosunGs.TTF")
    svg = ET.Element(f"{{{NS}}}svg", {"width": "320", "height": "226", "viewBox": "0 0 320 226", "role": "img"})
    ET.SubElement(svg, f"{{{NS}}}title").text = brand["videoTitle"] + " " + brand["videoUrl"]
    ET.SubElement(svg, f"{{{NS}}}desc").text = "기존 벡터 로고 아래에 조선궁서체 제목과 GitHub 주소. 배경 없음. 서체 저작권: (주)조선일보사."
    logo = ET.SubElement(svg, f"{{{NS}}}g", {"transform": "translate(126 0) scale(0.08640406607369759) translate(-627 -424)"})
    for child in ET.parse(ROOT / brand["logoPath"]).getroot():
        if child.tag == f"{{{NS}}}g":
            logo.append(child)
    for text, size, baseline, outline in [(brand["videoTitle"], 52, 166, "2"), (brand["videoUrl"], 17, 217, "1.4")]:
        paths = lettering(font, text, size, baseline)
        # A thin letter-shaped outline; no background rectangle or raster images.
        for attrs in [{"fill": "#151513", "stroke": "#f7f3eb", "stroke-width": outline, "stroke-linejoin": "round"}, {"fill": "#151513"}]:
            group = ET.SubElement(svg, f"{{{NS}}}g", attrs)
            for path in paths:
                ET.SubElement(group, f"{{{NS}}}path", {"d": path})
    output = ROOT / "docs/assets/odeduck-hero-brand.svg"
    ET.ElementTree(svg).write(output, encoding="unicode", xml_declaration=True)
    print(output)


if __name__ == "__main__":
    main()
