#!/usr/bin/env python3
"""Export the stored diagram HTML to PNG, after checking fonts and label bounds."""

import argparse
import json
from pathlib import Path
import sys
import xml.etree.ElementTree as ET
import re

ASSETS = Path(__file__).resolve().parents[2] / 'docs/assets'
DIAGRAMS = ('api-workflow', 'system-overview', 'goal-flow')

GEOMETRY = """() => {
  const box = e => {
    const r = e.getBBox();
    return {x:r.x, y:r.y, w:r.width, h:r.height};
  };
  const overlaps = (a,b) => a.x < b.x+b.w && a.x+a.w > b.x &&
                            a.y < b.y+b.h && a.y+a.h > b.y;
  const nodes = [...document.querySelectorAll('[data-node]')].map(e => {
    const [x,y,w,h] = e.dataset.bounds.split(',').map(Number);
    return {id:e.dataset.node, b:{x,y,w,h},
            texts:[...e.querySelectorAll('text')].map(t => ({text:t.textContent, b:box(t)}))};
  });
  const clipped = nodes.flatMap(n => n.texts.filter(t =>
    t.b.x < n.b.x+4 || t.b.x+t.b.w > n.b.x+n.b.w-4 ||
    t.b.y < n.b.y || t.b.y+t.b.h > n.b.y+n.b.h
  ).map(t => ({node:n.id, ...t})));
  const masks = [...document.querySelectorAll('[data-label]')].map(e =>
    ({id:e.dataset.label, b:box(e.querySelector('rect'))}));
  const collisions = masks.flatMap(m => nodes.filter(n => overlaps(m.b,n.b))
    .map(n => ({mask:m.id, node:n.id})));
  return {nodes:nodes.length, clipped, collisions};
}"""


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('diagrams', nargs='*', metavar='NAME',
                        help='api-workflow, system-overview, goal-flow (default: all)')
    parser.add_argument('--source-dir', type=Path, default=ASSETS)
    parser.add_argument('--output-dir', type=Path, default=ASSETS)
    parser.add_argument('--scale', type=int, choices=(1, 2, 3, 4), default=2)
    args = parser.parse_args()
    if unknown := set(args.diagrams) - set(DIAGRAMS):
        parser.error('unknown diagram: ' + ', '.join(sorted(unknown)))

    try:
        from playwright.sync_api import sync_playwright
    except ImportError:
        parser.exit(1, 'PNG export needs Playwright. See docs/assets/README.md for setup.\n')

    args.output_dir.mkdir(parents=True, exist_ok=True)
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch()
        try:
            for name in args.diagrams or DIAGRAMS:
                source = (args.source_dir / f'odeduck-{name}.html').resolve()
                match = re.search(r'<svg\b.*?</svg>', source.read_text(encoding='utf-8'), re.S)
                if not match:
                    raise ValueError(f'No SVG diagram in {source}')
                svg = ET.fromstring(match[0])
                _, _, width, height = map(float, svg.attrib['viewBox'].split())
                page = browser.new_page(
                    viewport={'width':int(width), 'height':int(height)},
                    device_scale_factor=args.scale,
                )
                remote = []

                def block_remote(route):
                    remote.append(route.request.url)
                    route.abort()

                page.route(re.compile(r'^https?://'), block_remote)
                page.goto(source.as_uri())
                page.evaluate('document.fonts.ready')
                fonts = page.evaluate("""() => [...document.fonts].every(f =>
                  f.family === 'Pretendard' && f.status === 'loaded') &&
                  document.fonts.size > 0""")
                geometry = page.evaluate(GEOMETRY)
                if not fonts or remote or geometry['clipped'] or geometry['collisions']:
                    raise ValueError(json.dumps(
                        {'diagram':name, 'fonts_loaded':fonts, 'remote_requests':remote,
                         **geometry}, ensure_ascii=False,
                    ))
                output = args.output_dir / source.with_suffix('.png').name
                page.locator('svg').screenshot(path=str(output), omit_background=True)
                print(f'{output.name}: {int(width*args.scale)}×{int(height*args.scale)}; '
                      'fonts and label bounds OK; no remote assets')
                page.close()
        finally:
            browser.close()


if __name__ == '__main__':
    try:
        main()
    except (ValueError, OSError) as error:
        sys.exit(str(error))
