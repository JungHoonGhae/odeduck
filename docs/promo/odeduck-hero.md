# 오데덕 README 히어로 애니메이션

안경을 쓰지 않은 아주 작은 오데덕이 거대한 캐비닛 사이를 달리고 뛰어넘으며 여러 서랍을 열고, 서로
멀리 떨어진 데이터 조각을 모아 하나의 연결망으로 잇는 10초 루프다. 기능 화면을 흉내 내기보다 “답은
한 분야에 모여 있지 않다”는 제품의 역할을 캐릭터의 행동으로 먼저 이해시키는 데 목적이 있다.

## 연출 원칙

- 로고를 단순히 좌우로 움직이지 않는다. 캐비닛 횡단→서랍 탐색→수집→연결→확인의 작은 사건을 만든다.
- 캐릭터와 거대한 캐비닛의 크기 차이로 9만 6천여 건을 뒤지는 탐색 규모를 직관적으로 보여준다.
- 캐릭터의 동작과 표정은 자유롭게 두되, 검정 잉크와 따뜻한 아이보리 중심의 기존 브랜드 인상은 유지한다.
- 영상 캐릭터에는 안경이나 별도 강조색을 추가하지 않고 원본의 검정 잉크와 따뜻한 아이보리 톤을 유지한다.
- 화면 속 문서는 특정 기관이나 수치를 사칭하지 않는 추상적인 데이터 조각으로 표현한다.
- 연결망은 답이나 인과관계가 아니라, 실제 키·명세·컬럼으로 확인할 후보를 뜻한다.
- README에서는 가벼운 GIF가 자동 재생되고, 클릭하면 더 선명한 MP4 원본이 열린다.

## 산출물

| 파일 | 용도 |
| --- | --- |
| `docs/assets/odeduck-hero.gif` | README 자동 재생 히어로 |
| `docs/assets/odeduck-hero.mp4` | 로고·궁서체를 합성한 10초, 16:9, 720p H.264 영상. LinkedIn 업로드 겸용 |
| `docs/assets/odeduck-hero-poster.webp` | 정적 포스터·영상 폴백 |
| `docs/assets/odeduck-hero-source.mp4` | 합성 전 최초 생성 영상. 다시 출력할 때 사용 |
| `docs/assets/odeduck-hero-brand.svg` | 투명 배경의 벡터 로고·제목·GitHub 주소 |

Higgsfield Seedance 2.0 Mini로 만든 최초 영상은 source 파일로 보존한다. 영상 속 뛰어다니는 캐릭터는
수정하지 않았다. 좌측 하단에 기존 안경 쓴 빼꼼 로고, `오.데.덕.`, GitHub 주소를 위에서 아래로 왼쪽 정렬했다.
브랜드 영역은 260×150px이며 제목은 36px, 주소는 14px로 작게 유지한다.
배경 박스와 부제는 없다. 서랍 위에서도 글자를 읽을 수 있도록 글자 모양의 얇은 외곽선만 사용한다.
로고와 글자는 SVG 경로이며, 영상 합성 시에만 투명 RGBA로 변환한다. 음성은 없다.

글꼴은 개인·기업에 무료 사용과 재배포가 허용된 **조선궁서체**다.
TTF를 변경 없이 사용하며 [글꼴·이용 조건·출처](../../scripts/promo/fonts/)를 함께 보관한다.
영상에 들어가는 문구는 [브랜드 설정](../brand/brand.json)에서 관리한다.

정확한 제품 주장은 README의 카탈로그 수치, 경쟁 워크플로 감사, 실제 명세·컬럼 검증 설명이 맡고,
이 애니메이션은 그 앞에서 오데덕의 성격과 역할을 전달한다.

## 다시 출력하기

저장소 루트에서 Python 3, fontTools, ImageMagick(SVG 지원), FFmpeg(libx264, libwebp 포함)를 사용한다.

```sh
python3 -m pip install -r scripts/promo/requirements.txt
python3 scripts/promo/render.py
# 다른 FFmpeg 실행 파일 사용:
python3 scripts/promo/render.py --ffmpeg /path/to/ffmpeg
```

원본과 브랜드 설정으로 MP4, 무한 반복 GIF(800px, 10fps, 64색), 포스터를 함께 만든다.
GIF는 README 자동 재생용이고, LinkedIn에는 `odeduck-hero.mp4`를 업로드한다.

대안 시안과 생성 기록은 Git에서 제외된 로컬 `promo-studio/`에 보존한다. README에는 로고가 들어간
최종 GIF와 MP4를 노출한다. LinkedIn 공개 글은 [게시글 초안](linkedin-launch.md)에 있다.
