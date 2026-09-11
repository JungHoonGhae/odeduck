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
license_text = (HERE / 'fonts/OFL.txt').read_text(encoding='utf-8').rstrip().replace('--', '—')
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
      'price':'<path d="M4 28 L32 4 L60 28 M12 24 V60 H52 V24 M24 60 V40 H40 V60"/>',
      'shops':'<path d="M8 8 H56 L64 28 H0 Z M8 28 V60 H56 V28 M24 60 V40 H40 V60 M16 8 L12 28 M32 8 V28 M48 8 L52 28"/>',
      'soil':'<path d="M4 40 H60 M8 48 H24 M36 48 H56 M16 60 H48 M32 40 V24 M32 28 C8 28 8 8 8 8 C32 8 32 20 32 28 M32 24 C56 24 56 4 56 4 C32 4 32 16 32 24"/>',
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


def build_bottlenecks():
    # Manual user journey, not measured time savings or a claim of distinct keys per API.
    s = 'odeduck-manual-bottlenecks'
    b = text(72, 80, '공공데이터를 직접 찾아 쓰려면', 24, 600, MUTED)
    b += text(72, 140, '가격 비교를 하려는데, 준비부터 막힙니다.', 40, 700)
    b += mascot(1096, 56, 104)
    b += edge(s, 'M424 388 H464') + edge(s, 'M816 388 H856')

    search = plate(72, 200, 352, 344)
    search += text(104, 248, '01  검색어부터 막힘', 28, 700)
    search += plate(100, 280, 296, 64) + icon('inspect', 116, 296, 32)
    search += text(164, 320, '경매? 공매? 매각?', 24, 600)
    search += edge(s, 'M348 344 V376 A8 8 0 0 1 340 384 H164 A8 8 0 0 1 156 376 V344', dashed=True)
    search += text(248, 428, '검색어 바꾸고, 다시 찾고', 24, 500, INK, 'middle')
    search += text(248, 500, '있는 줄도 모르면 놓칩니다', 24, 700, INK, 'middle')
    b += node('search-friction', 72, 200, 352, 344, search)

    apply = plate(464, 200, 352, 344)
    apply += text(496, 248, '02  서비스마다 반복', 28, 700)
    for y, name in [(280, 'API A'), (340, 'API B'), (400, 'API C')]:
        apply += icon('apply', 496, y + 8, 32)
        apply += text(544, y + 32, name, 24, 700)
        apply += text(636, y + 32, '신청 → 승인 확인', 20, 500)
    apply += text(640, 500, '필요한 서비스마다 따로 신청', 24, 700, INK, 'middle')
    b += node('application-friction', 464, 200, 352, 344, apply)

    access = plate(856, 200, 352, 344)
    access += text(888, 248, '03  승인 후에도 설정', 28, 700)
    for y, name, label in [(284, 'key', '인증키 복사'), (348, 'inspect', '입력 방식 확인'), (412, 'call', '호출 설정')]:
        access += icon(name, 896, y, 36) + text(956, y + 28, label, 28, 600)
    access += text(1032, 500, '첫 조회까지 직접 챙깁니다', 24, 700, INK, 'middle')
    b += node('access-friction', 856, 200, 352, 344, access)

    b += node('repeat', 72, 584, 1136, 88,
              plate(72, 584, 1136, 88, INK)
              + text(640, 640, '자료가 하나 더 필요하면, 이 절차도 한 번 더.', 32, 700, CREAM, 'middle'))
    write(s, '비교를 시작하기 전에 반복하는 공공데이터 이용 절차',
          '검색어를 바꾸며 자료를 찾고, 필요한 미신청 API마다 활용신청과 승인 확인을 반복한 뒤 인증키 입력 방식과 호출을 직접 설정해야 하므로 여러 자료를 비교하기 전에 준비 작업이 쌓인다.',
          720, b, 'manual user journey / slide-16x9 / branded variation')


def build_goal():
    # Explain the output with a clearly fictional table, not an end-to-end success claim.
    s = 'odeduck-goal-flow'
    b = text(72, 72, '찾은 자료로 비교표 만들기 · 실험 기능', 24, 600, MUTED)
    b += text(72, 128, '이 공매 아파트, 비슷한 거래보다 싼가?', 40, 700)
    b += mascot(1096, 48, 104)
    b += text(72, 176, '작동 방식을 설명하는 가상 예시', 24, 500, MUTED)

    b += edge(s, 'M344 320 V368') + edge(s, 'M936 320 V368')
    b += edge(s, 'M640 448 V504')
    for x, name, label, fields in [
        (72, 'price', '공매 물건', '최소입찰가 · 주소 · 면적'),
        (664, 'catalog', '실거래 기록', '거래금액 · 주소 · 면적 · 거래일'),
    ]:
        b += node(name, x, 216, 544, 104,
                  plate(x, 216, 544, 104)
                  + icon(name, x + 24, 244, 48)
                  + text(x + 96, 260, label, 32, 700)
                  + text(x + 96, 300, fields, 24, 500, MUTED))

    b += node('match', 72, 368, 1136, 80,
              plate(72, 368, 1136, 80, INK)
              + icon('join', 100, 388, 40, CREAM)
              + text(176, 420, '단지 · 면적 · 거래 시점을 맞춰, 비교할 거래를 고릅니다', 28, 700, CREAM))

    table = plate(72, 504, 1136, 216)
    for x, label in [(104, '공매 물건'), (344, '최소입찰가'), (608, '비교 거래가'), (904, '자료 확인')]:
        table += text(x, 548, label, 24, 600, MUTED)
    table += f'<path d="M96 568 H1184 M96 640 H1184" stroke="{MUTED}" stroke-width="1"/>'
    for y, values in [(616, ['물건 A', '3억 원', '3.4억 원', '비교 자료 있음']),
                      (688, ['물건 B', '2억 원', '—', '비교 자료 부족'])]:
        for x, value in zip([104, 344, 608, 904], values):
            table += text(x, y, value, 28, 700)
    b += node('comparison', 72, 504, 1136, 216, table)
    b += text(72, 768, '표와 함께  사용한 출처 · 비교 조건 · 확인하지 못한 부분을 남깁니다', 28, 600)
    b += text(72, 816, '맞는 자료가 없으면 다시 찾고, 끝내 찾지 못한 부분은 미완료로 남깁니다.', 24, 500, MUTED)
    write(s, '공매 가격과 실거래가로 비교표를 만드는 실험 기능',
          '설명용 가상 예시에서 공매 물건과 실거래 기록의 단지·면적·거래 시점을 맞춰 가격을 나란히 놓고, 비교에 쓴 출처와 조건 및 맞는 거래를 찾지 못한 물건을 함께 표시하며 부족한 자료는 다시 찾거나 미완료로 남긴다.',
          864, b, 'data comparison / doc-wide / fictional worked example')


def build_connections():
    # A request example grounded in the recorded auction discovery evaluation.
    # Branches represent data roles, not simultaneous execution or verified joins.
    s = 'odeduck-unexpected-connections'
    b = text(72, 80, '공공데이터는 오데덕에게 맡기세요.', 40, 700)
    b += mascot(1096, 40, 104)

    # Pain now has its own first slide; this slide starts with the user's goal.
    b += text(72, 144, '자료 이름을 몰라도, 이렇게 맡겨보세요.', 28, 600)
    for x in (248, 640, 1032):
        b += edge(s, f'M{x} 336 V392')
        b += edge(s, f'M{x} 552 V600')
    b += edge(s, 'M640 664 V720')

    b += node('question', 72, 232, 1136, 104,
              plate(72, 232, 1136, 104)
              + icon('chat', 100, 256, 48)
              + text(176, 272, '싸게 나온 부산 공매 부동산, 정말 싼 걸까?', 28, 700)
              + text(176, 312, '비교할 가격과 놓치기 쉬운 위험을 찾아줘.', 28, 700))

    for x, name, title, perspective, unexpected in [
        (72, 'price', '아파트 실거래가', '가격을 비교할 기준', False),
        (464, 'shops', '상권 변화', '주변 수요를 볼 단서', True),
        (856, 'soil', '토양오염 조사', '환경을 확인할 단서', True),
    ]:
        fill, ink = (INK, CREAM) if unexpected else (CREAM, INK)
        b += node(name, x, 392, 352, 160,
                  plate(x, 392, 352, 160, fill)
                  + text(x + 28, 424, '뜻밖의 연결 후보' if unexpected else '먼저 떠올릴 자료',
                         16, 500, ink)
                  + icon(name, x + 28, 448, 48, ink)
                  + text(x + 96, 484, title, 28, 700, ink)
                  + text(x + 28, 524, perspective, 20, 500, ink))

    b += node('access', 72, 600, 1136, 64,
              plate(72, 600, 1136, 64)
              + icon('key', 104, 616, 32)
              + text(160, 640, '오데덕이  활용신청 · 승인 확인 · 인증키 자동 입력 · 조회', 28, 600))
    b += node('perspectives', 72, 720, 1136, 112,
              plate(72, 720, 1136, 112)
              + icon('report', 104, 748, 48)
              + text(184, 764, '함께 볼 자료 + 연결 이유 + 출처', 28, 700)
              + text(184, 804, '가격을 넘어, 상권과 환경까지 살펴볼 관점', 20, 500, MUTED))
    b += text(72, 880, '발견 기록을 바탕으로 한 요청 예시 · 연결 전 지역·기간·대상 확인',
              20, 500, MUTED)
    write(s, '한 질문에서 예상 밖의 연결까지',
          '공매 부동산을 살펴보려는 질문에서 실거래가와 함께 부산 상권 변화, 토양오염 조사 자료를 연결 후보로 찾는다. '
          '필요한 API는 신청과 승인 확인, 인증키 입력을 거쳐 조회하고 함께 검토할 관점과 출처를 얻는다. '
          '가지들은 동시 실행이나 실제 데이터 결합의 완료를 뜻하지 않으며 지역·기간·대상 확인이 필요하다.',
          928, b, 'request-to-discovery flow / doc-wide / branded variation')


DIAGRAMS = {
    'manual-bottlenecks': build_bottlenecks,
    'unexpected-connections': build_connections,
    'api-workflow': build_workflow,
    'system-overview': build_architecture,
    'goal-flow': build_goal,
}


def main():
    global OUT, FETCH_FONTS
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('diagrams', nargs='*', metavar='NAME',
                        help=', '.join(DIAGRAMS) + ' (default: all)')
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
