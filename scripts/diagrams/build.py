#!/usr/bin/env python3
"""Build the README diagrams from their editable layouts and local brand assets."""

import argparse
import base64
import concurrent.futures
import hashlib
from html import escape
from pathlib import Path
import re
from urllib.request import urlopen
import xml.etree.ElementTree as ET

ET.register_namespace('', 'http://www.w3.org/2000/svg')
ROOT = Path(__file__).resolve().parents[2]
HERE = Path(__file__).resolve().parent
OUT=ROOT/'docs/assets'
CREAM='#F7F3EB'; INK='#151513'; MUTED='#625F57'
font_css = (HERE / 'fonts/pretendard-dynamic.css').read_text(encoding='utf-8')
license_text = (HERE / 'fonts/OFL.txt').read_text(encoding='utf-8').replace('--', '—')
cache = HERE / 'fonts'
FETCH_FONTS = False
FONT_BASE_URL = 'https://raw.githubusercontent.com/orioncactus/pretendard/v1.3.9/packages/pretendard/dist/web/variable/'

def icon(name,x,y,size=64,color=INK):
    # Simple purpose-drawn 64px line drawings; no external icon/image dependencies.
    forms={
      'catalog':'<rect x="8" y="8" width="40" height="48" rx="8"/><path d="M16 8 V0 H56 V48 H48 M20 24 H36 M20 36 H36 M20 48 H28"/>',
      'inspect':'<rect x="4" y="4" width="36" height="48" rx="8"/><path d="M12 16 H28 M12 28 H20"/><circle cx="40" cy="40" r="12"/><path d="M48 48 L60 60"/>',
      'apply':'<rect x="12" y="8" width="40" height="48" rx="8"/><rect x="24" y="0" width="16" height="16" rx="4" fill="currentColor"/><path d="M24 28 H40 M24 40 H40 M32 56 V64 M24 56 L32 64 L40 56"/>',
      'approved':'<circle cx="32" cy="32" r="28"/><path d="M16 32 L28 44 L48 20"/>',
      'key':'<circle cx="20" cy="24" r="16"/><circle cx="16" cy="20" r="4" fill="currentColor" stroke="none"/><path d="M32 36 L56 60 M44 48 L52 40 M52 56 L60 48"/>',
      'call':'<path d="M4 16 H16 V8 H48 V16 H60 V48 H48 V56 H16 V48 H4 Z M24 24 L16 32 L24 40 M40 24 L48 32 L40 40"/>',
      'browser':'<rect x="0" y="8" width="64" height="48" rx="8"/><path d="M0 24 H64 M12 16 H12 M24 16 H24"/>',
      'terminal':'<rect x="0" y="8" width="64" height="48" rx="8"/><path d="M12 24 L24 32 L12 40 M32 44 H48"/>',
      'chat':'<path d="M8 8 H56 A8 8 0 0 1 64 16 V40 A8 8 0 0 1 56 48 H28 L12 60 V48 H8 A8 8 0 0 1 0 40 V16 A8 8 0 0 1 8 8 Z M16 24 H48 M16 36 H36"/>',
      'file':'<path d="M12 4 H36 L52 20 V60 H12 Z M36 4 V20 H52 M20 32 H44 M20 44 H44"/>',
      'portal':'<path d="M4 20 L32 4 L60 20 Z M8 56 H56 M8 24 V52 M24 24 V52 M40 24 V52 M56 24 V52"/>',
      'globe':'<circle cx="32" cy="32" r="28"/><ellipse cx="32" cy="32" rx="12" ry="28"/><path d="M4 32 H60 M12 16 H52 M12 48 H52"/>',
      'join':'<rect x="0" y="8" width="24" height="48" rx="4"/><rect x="40" y="8" width="24" height="48" rx="4"/><path d="M0 24 H24 M40 24 H64 M12 40 H52"/><circle cx="32" cy="40" r="8" fill="currentColor"/>',
      'review':'<rect x="8" y="0" width="40" height="52" rx="8"/><path d="M16 12 H36 M16 24 H28"/><circle cx="44" cy="44" r="16"/><path d="M36 44 L44 52 L56 36"/>',
      'report':'<path d="M8 4 H44 V60 H8 Z M44 12 H56 V52 H44 M16 16 H32 M16 28 H36 M16 48 V40 M24 48 V32 M32 48 V36"/>',
    }
    return f'<g aria-hidden="true" transform="translate({x} {y}) scale({size/64})" color="{color}" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round">{forms[name]}</g>'

def text(x,y,value,size=24,weight=600,fill=INK,anchor=None):
    a=f' text-anchor="{anchor}"' if anchor else ''
    return f'<text x="{x}" y="{y}" font-size="{size}" font-weight="{weight}" fill="{fill}"{a}>{escape(value)}</text>'

def mascot(x,y,w=120):
    root=ET.fromstring((ROOT/'docs/assets/brand-symbol.svg').read_text())
    children=''.join(ET.tostring(c,encoding='unicode') for c in root if c.tag.endswith('g'))
    # The existing mark remains intact, just positioned on the diagram canvas.
    return f'<g aria-hidden="true" transform="translate({x} {y}) scale({w/2048})">{children}</g>'

def edge(slug,path,color=INK,dashed=False,marker=True):
    dash=' stroke-dasharray="4 8"' if dashed else ''
    marker_id='light' if color==CREAM else 'arrow'
    tip=f' marker-end="url(#{slug}-{marker_id})"' if marker else ''
    return f'<path data-edge="" d="{path}" fill="none" stroke="{color}" stroke-width="2" stroke-linecap="round"{dash}{tip}/>'

def plate(x,y,w,h=80,fill=CREAM,stroke=INK):
    return f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="24" fill="{fill}" stroke="{stroke}" stroke-width="2"/>'

def node(name,x,y,w,h,content):
    return f'<g data-node="{name}" data-bounds="{x},{y},{w},{h}">{content}</g>'

def font_faces(svg):
    r=ET.fromstring(svg)
    chars={ord(c) for e in r.iter() if e.tag.endswith('text') for c in ''.join(e.itertext())}
    faces=[];covered=set()
    for face in re.findall(r'@font-face\s*\{[^}]+\}',font_css):
        points=set()
        for lo,hi in re.findall(r'U\+([0-9a-f]+)(?:-([0-9a-f]+))?',re.search(r'unicode-range:\s*([^;]+)',face)[1],re.I):
            points.update(range(int(lo,16),int(hi or lo,16)+1))
        if points & chars:faces.append(face);covered.update(points)
    assert chars<=covered,chars-covered
    def fetch(face):
        relative=re.search(r'url\(\./([^)]*)\)',face)[1];p=cache/Path(relative).name
        if not p.exists():
            if not FETCH_FONTS:
                raise ValueError(
                    f'Missing font subset {p.name}. New text needs an additional official '
                    'Pretendard subset. Run build.py --fetch-fonts, then commit the added font.'
                )
            with urlopen(FONT_BASE_URL + relative, timeout=30) as response:
                content = response.read()
            if not content.startswith(b'wOF2'):
                raise ValueError(f'Invalid WOFF2 download: {p.name}')
            p.write_bytes(content)
        content = p.read_bytes()
        if not content.startswith(b'wOF2'):
            raise ValueError(f'Invalid local WOFF2 font: {p.name}')
        face=face.replace("'Pretendard Variable'","'Pretendard'").replace('font-display: swap','font-display: block')
        face=re.sub(r'url\([^)]*\)',"url('data:font/woff2;base64,"+base64.b64encode(content).decode()+"')",face)
        return f'/* Official Pretendard v1.3.9 {p.name}; SHA-256 {hashlib.sha256(content).hexdigest()} */\n'+face
    with concurrent.futures.ThreadPoolExecutor(max_workers=6) as p:return '\n'.join(p.map(fetch,faces))

def write(slug,title,desc,h,body,kind):
    markers=''.join(f'<marker id="{slug}-{suffix}" markerWidth="8" markerHeight="8" refX="8" refY="4" orient="auto" markerUnits="userSpaceOnUse"><path d="M0 0 L8 4 L0 8 Z" fill="{color}"/></marker>' for suffix,color in [('arrow',INK),('light',CREAM),('link',MUTED)])
    svg=f'''<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1280 {h}" width="1280" height="{h}" role="img" aria-labelledby="{slug}-title {slug}-desc">
<title id="{slug}-title">{title}</title>
<desc id="{slug}-desc">{desc}</desc>
<defs>{markers}</defs>
<rect width="1280" height="{h}" fill="{CREAM}"/>
<rect x="24" y="24" width="1232" height="{h-48}" rx="28" fill="none" stroke="{INK}" stroke-width="2"/>
{body}
</svg>'''
    html=f'''<!doctype html>
<html lang="ko"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{title}</title>
<!-- Diagram Design / {kind}, 1280 × {h}, static editorial variation.
Style follows existing odeduck-linkedin-demo.svg / odeduck-before-after.svg at the user's request:
cream paper, near-black ink, rounded frames and the unchanged character. These explicit brand choices
override the skill's default palette/radius/type ramp. Korean type: user-selected Pretendard.
Long command details remain in README. Generated by scripts/diagrams/build.py.
Edit that layout source, then rebuild HTML and PNG; see docs/assets/README.md.
The font safety checker rejects embedded font URLs; those are intentional, fully local font data.
Official unmodified Pretendard subsets cover all visible glyphs. Add matching official subsets when
new text introduces uncovered glyphs. No external assets, scripts, font services or stylesheets load.
{license_text}
-->
<style>{font_faces(svg)}
*{{box-sizing:border-box}}body{{margin:0;background:{CREAM};color:{INK}}}main{{max-width:1280px;margin:auto}}svg{{display:block;width:100%;height:auto;font-family:'Pretendard','Apple SD Gothic Neo','Malgun Gothic',sans-serif}}
</style></head><body><main>{svg}</main></body></html>'''
    html='\n'.join(l.rstrip() for l in html.splitlines())+'\n'
    (OUT/(slug+'.html')).write_text(html, encoding='utf-8')
    print(slug,len(html.encode()),'bytes')



def build_workflow():
    # 1. Stable product capability: explicit human login, then agent-led portal automation.
    s='odeduck-api-workflow'
    b=text(72,80,'오데덕이 하는 일',16,700)+text(72,136,'질문에서 첫 호출까지.',40,700)+text(72,176,'data.go.kr REST',20,400,fill=MUTED)+mascot(1092,60,112)
    b+=f'<rect x="448" y="248" width="760" height="288" rx="28" fill="{INK}"/>'
    b+=text(72,288,'에이전트가 찾고 검사',20,600)+text(480,288,'오데덕 자동 처리',20,600,CREAM)
    b+=edge(s,'M952 224 V248')
    b+=edge(s,'M192 384 H288')+edge(s,'M384 384 H448',marker=False)+edge(s,'M448 384 H480',CREAM)
    for x in [576,768,960]:b+=edge(s,f'M{x} 384 H{x+96}',CREAM)
    b+=node('login',752,168,400,56,plate(752,168,400,56)+icon('browser',776,180,32)+text(828,204,'사람은 로그인 1회',24,600))
    for x,name,label in [(144,'catalog','탐색'),(336,'inspect','검사'),(528,'apply','활용신청'),(720,'approved','승인 확인'),(912,'key','키 자동 주입'),(1104,'call','호출')]:
        color=INK if x<448 else CREAM
        b+=node(name,x-80,340,160,152,icon(name,x-36,348,72,color)+text(x,476,label,24,700,color,'middle'))
    b+=f'<path d="M72 560 H1208" stroke="{INK}" stroke-width="2" stroke-dasharray="4 8"/>'
    b+=text(72,592,'온비드 · 나라장터 · 도매시장 · 중소기업 지원사업 — 실계정 신청·호출 검증',20,500)
    write(s,'질문에서 첫 호출까지','에이전트가 데이터를 탐색하고 검사하며, 사람의 정부 SSO 로그인 한 번 이후 오데덕이 필요한 REST 활용신청, 승인 확인, 인증키 주입과 호출을 잇는다.',640,b,'workflow / swimlane')


def build_architecture():
    # 2. Shared implementation and provider boundaries; FILE does not go through API application.
    s='odeduck-system-overview'
    b=text(72,80,'전체 아키텍처',16,700)+text(72,136,'CLI든, MCP든. 같은 엔진으로.',40,700)+mascot(1092,56,112)
    b+=f'<rect x="328" y="208" width="488" height="416" rx="28" fill="{INK}"/>'
    b+=text(360,252,'odeduck · 공통 Go 엔진',24,700,CREAM)
    b+=edge(s,'M264 332 H328',marker=False)+edge(s,'M328 332 H360',CREAM)
    b+=edge(s,'M264 508 H288 A8 8 0 0 0 296 500 V376 A8 8 0 0 1 304 368 H328',marker=False)+edge(s,'M328 368 H360',CREAM)
    b+=edge(s,'M520 352 H600',CREAM)+edge(s,'M692 400 V472',CREAM)
    b+=edge(s,'M784 336 H816',CREAM,marker=False)+edge(s,'M816 336 H848 A8 8 0 0 0 856 328 V296 A8 8 0 0 1 864 288 H976')
    b+=edge(s,'M784 512 H816',CREAM,marker=False)+edge(s,'M816 512 H872 A8 8 0 0 0 880 504 V472 A8 8 0 0 1 888 464 H976')
    b+=edge(s,'M784 548 H816',CREAM,marker=False)+edge(s,'M816 548 H904 A8 8 0 0 1 912 556 V600 A8 8 0 0 0 920 608 H976')
    for name,y,label,sub in [('terminal',280,'CLI','사람'),('chat',456,'MCP','AI 에이전트')]:
        b+=node(name,72,y,192,104,plate(72,y,192,104)+icon(name,96,y+28,48)+text(168,y+40,sub,16,500,MUTED)+text(168,y+76,label,28,700))
    b+=node('catalog',360,304,160,96,plate(360,304,160,96,CREAM,CREAM)+icon('catalog',424,316,32)+text(440,384,'카탈로그',24,700,INK,'middle'))
    b+=node('inspect',600,304,184,96,plate(600,304,184,96,CREAM,CREAM)+icon('inspect',676,316,32)+text(692,384,'원천 검사',24,700,INK,'middle'))
    b+=node('access',360,472,424,104,plate(360,472,424,104,CREAM,CREAM)+icon('key',384,500,48)+text(464,532,'신청 · 인증 · 호출',28,700))
    for name,y,title,sub in [('file',248,'FILE · STD','파일 · 표준표 관찰'),('portal',424,'data.go.kr','REST API'),('globe',568,'외부 제공기관','검증된 LINK')]:
        b+=node(name,976,y,232,80,plate(976,y,232,80)+icon(name,992,y+20,40)+text(1052,y+36,title,24,700)+text(1052,y+60,sub,16,500,MUTED))
    b+=text(72,680,'로컬 보관: 카탈로그 · 로그인 세션 · 인증키 · 연결 근거',20,500)
    write(s,'CLI와 MCP가 공유하는 오데덕 아키텍처','사람의 CLI와 AI 에이전트의 MCP가 공통 Go 엔진의 카탈로그와 원천 검사를 사용하고, 파일·표준데이터는 직접 관찰하며 API는 신청·인증·호출을 거쳐 data.go.kr REST 또는 검증된 외부 제공기관에 접근한다.',720,b,'architecture / doc-wide')


def build_goal():
    # 3. Experimental goal loop: fewer words, no claim of generic autonomous completion.
    s='odeduck-goal-flow'
    b=text(72,72,'목표 기반 연결·분석',16,700)+text(72,128,'근거가 부족하면, 다시 찾는다.',40,700)
    b+=plate(1024,64,184,48)+text(1116,96,'실험 기능',20,700,INK,'middle')
    b+=edge(s,'M232 264 H416')+edge(s,'M512 264 H704')+edge(s,'M800 264 H1024')
    b+=edge(s,'M752 224 V192 A8 8 0 0 0 744 184 H192 A8 8 0 0 0 184 192 V224',dashed=True)
    b+=edge(s,'M752 344 V384',dashed=True)
    b+=f'<g data-label="replan"><rect x="412" y="148" width="112" height="28" fill="{CREAM}"/>'+text(468,168,'재탐색',20,600,INK,'middle')+'</g>'
    b+=f'<g data-label="pass"><rect x="880" y="224" width="64" height="32" fill="{CREAM}"/>'+text(912,248,'통과',20,600,INK,'middle')+'</g>'
    for x,name,label in [(184,'inspect','검색·검사'),(464,'join','연결·계산'),(752,'review','근거 검토'),(1072,'report','결과와 출처')]:
        b+=node(name,x-104,224,208,120,icon(name,x-36,228,72)+text(x,336,label,28,700,INK,'middle'))
    b+=node('incomplete',668,384,168,48,plate(668,384,168,48)+text(752,416,'미완료',24,600,INK,'middle'))
    write(s,'근거가 부족하면 다시 찾는 목표 실행','검색·검사한 원천을 연결·계산하고 근거를 검토하며, 근거가 부족하면 예산 안에서 재탐색하고 허용된 검토를 통과하면 결과와 출처를 반환하며 더 진행할 수 없으면 미완료로 남긴다.',480,b,'goal flow / editorial')


DIAGRAMS = {
    'api-workflow': build_workflow,
    'system-overview': build_architecture,
    'goal-flow': build_goal,
}


def main():
    global OUT, FETCH_FONTS
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('diagrams', nargs='*', metavar='NAME',
                        help='api-workflow, system-overview, goal-flow (default: all)')
    parser.add_argument('--output-dir', type=Path, default=OUT,
                        help='HTML output directory (default: repository docs/assets)')
    parser.add_argument('--fetch-fonts', action='store_true',
                        help='download missing Pretendard v1.3.9 subsets into scripts/diagrams/fonts')
    args = parser.parse_args()
    if unknown := set(args.diagrams) - DIAGRAMS.keys():
        parser.error('unknown diagram: ' + ', '.join(sorted(unknown)))
    OUT = args.output_dir.resolve()
    OUT.mkdir(parents=True, exist_ok=True)
    FETCH_FONTS = args.fetch_fonts
    try:
        for name in args.diagrams or DIAGRAMS:
            DIAGRAMS[name]()
    except (ValueError, OSError) as error:
        parser.exit(1, f'{error}\n')


if __name__ == '__main__':
    main()
