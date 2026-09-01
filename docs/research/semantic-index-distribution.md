# 9.6만 건 로컬 시맨틱 인덱스 배포·갱신 조사

조사일: 2026-09-01

## 결론

95,956건은 일반적인 개인용 컴퓨터에서 검색하기 어려운 규모가 아니다. 문제는 검색보다 **최초 문서
95,956건을 각 사용자 컴퓨터에서 다시 임베딩하는 시간**이다. 모든 사용자에게 `semantic-build`를
요구하면 시맨틱 검색의 품질 이득과 무관한 설치 병목을 만든다.

권장 기본 경로는 다음과 같다.

1. 프로젝트가 기본 모델로 만든 **공식 사전 구축 인덱스**를 카탈로그 스냅샷과 함께 배포한다.
2. 사용자는 검증된 번들을 내려받아 즉시 검색하고, 전체 로컬 빌드는 커스텀 카탈로그용 고급 기능으로
   남긴다.
3. 카탈로그 갱신 때는 전체 snapshot digest 하나로 인덱스를 폐기하지 않고, 정규화된 임베딩 입력의
   문서별 hash로 변하지 않은 벡터를 재사용한다.
4. 저장 형식은 `gob + [][]float32`에서 versioned manifest와 연속형 vector shard로 옮긴다. 처음에는
   exact flat scan을 유지하고, 파일은 mmap할 수 있게 만든다.
5. 256차원 또는 int8은 한국 공공데이터 봉인 질의에서 품질 차이를 측정한 뒤 선택한다. ANN이나 별도
   vector DB는 9.6만 건이라는 숫자만으로 도입하지 않는다.

사전 구축 인덱스가 문서 임베딩 작업은 없애지만, 사용자의 새 질의를 같은 벡터 공간으로 바꾸는
**query embedder는 여전히 필요하다**. Ollama를 설치하지 않은 사용자는 지금처럼 model-planned lexical
검색을 쓰고, 설치한 사용자는 같은 모델·차원·prompt recipe로 시맨틱 검색을 추가하는 구조가 맞다.

## 현재 규모와 일반 컴퓨터 적합성

아래는 파일 header, PK, hash, 정렬 구조를 제외한 순수 vector payload 계산값이다.

| 형식 | 768차원 | 512차원 | 256차원 | 128차원 |
|---|---:|---:|---:|---:|
| float32 | 281.12 MiB | 187.41 MiB | 93.71 MiB | 46.85 MiB |
| float16 | 140.56 MiB | 93.71 MiB | 46.85 MiB | 23.43 MiB |
| int8 | 70.28 MiB | 46.85 MiB | 23.43 MiB | 11.71 MiB |

현재 카탈로그는 약 99MB이고 기본 Ollama 모델은 약 239MB다. 따라서 768d float32까지 포함해도 디스크
용량 자체는 일반적인 컴퓨터에서 무리가 아니다. [Ollama 공식 모델 목록](https://ollama.com/library/embeddinggemma/tags)은
현재 기본 Q4 모델을 239MB로 표시한다. 여기서 Q4는 **모델 가중치의 양자화**이고, 저장하는 출력 벡터가
자동으로 4-bit가 된다는 뜻은 아니다.

현재 구현의 더 큰 문제는 다음 두 가지다.

- `[][]float32`를 gob으로 복원하므로 행마다 allocation이 생기고 전체 파일을 heap에 올린다.
- `CatalogDigest`가 `SyncedAt`과 전체 entry JSON을 포함한다. 한 항목이나 동기화 시각만 달라져도 기존
  95,955개 벡터까지 모두 stale이 된다.

따라서 “고사양 Mac Studio에서 전체 빌드가 되는가”보다 “8GB RAM 노트북이 완성된 index를 복사 없이
열 수 있고, 다음 갱신에서 몇 건만 다시 계산하는가”를 지원 기준으로 삼아야 한다.

## 유사 구현에서 가져올 패턴

| 구현 | 검증된 패턴 | oddsock에 적용할 부분 |
|---|---|---|
| Hugging Face Hub | revision별 snapshot, content blob 재사용, 파일·chunk cache로 같은 데이터를 다시 받지 않음. [공식 cache 구조](https://huggingface.co/docs/huggingface_hub/guides/manage-cache), [snapshot download](https://huggingface.co/docs/huggingface_hub/en/package_reference/file_download) | 논리 버전과 실제 blob을 분리하고, 동일 hash shard를 버전 사이에서 재사용 |
| Apache Lucene | 새 파일을 먼저 쓰고 이를 가리키는 새 `segments_N`을 commit point로 공개한다. 이전 checkpoint 전용 파일은 나중에 정리한다. [IndexWriter source의 checkpoint 설명](https://github.com/apache/lucene/blob/main/lucene/core/src/java/org/apache/lucene/index/IndexWriter.java) | immutable vector shard를 먼저 검증하고 마지막에 `CURRENT` manifest만 교체; 이전 한 세대 보존 |
| OCI Distribution | blob과 manifest를 digest로 주소화하고 client가 받은 body digest를 검증한다. Range 요청도 권고한다. [OCI Distribution Specification](https://github.com/opencontainers/distribution-spec/blob/main/spec.md) | 장기적으로 content-addressed shard 저장소와 재개 가능한 다운로드에 사용 |
| TUF | target metadata에 version, length, hashes를 기록하고 consistent snapshot은 hash가 포함된 filename을 사용한다. [TUF specification](https://github.com/theupdateframework/specification/blob/master/tuf-spec.md) | manifest rollback 방지와 길이·SHA-256 검증 원칙; 당장은 전체 TUF 구현보다 작은 signed manifest부터 시작 |
| USearch | index 저장·복원, 파일을 메모리에 전부 복사하지 않는 view/mmap, int8 등 여러 저장 dtype과 Go binding을 제공한다. 또한 ANN은 주로 수백만 항목부터 유용하고 작은 collection에는 direct search가 가능하다고 설명한다. [USearch repository](https://github.com/unum-cloud/USearch), [Go binding](https://github.com/unum-cloud/USearch/blob/main/golang/README.md) | mmap/quantized backend 후보. 단, native binding 배포비용 때문에 먼저 자체 flat format을 측정 |
| Faiss | exact 결과가 필요하면 Flat을 기준선으로 두고, fp16·SQ8·PQ 등 메모리/정확도 절충을 제공한다. [index 선택 지침](https://github.com/facebookresearch/faiss/wiki/Guidelines-to-choose-an-index), [encoding별 크기](https://github.com/facebookresearch/faiss/wiki/The-index-factory) | quantization과 ANN의 Recall 기준선을 반드시 float32 exact 결과로 유지 |
| LlamaIndex | ingestion cache와 docstore hash를 사용하고, upsert 시 같은 document hash는 재처리하지 않는다. [공식 pipeline source](https://github.com/run-llama/llama_index/blob/main/llama-index-core/llama_index/core/ingestion/pipeline.py) | PK가 아니라 실제 embedding input hash를 벡터 cache key로 사용 |

이 사례들의 공통점은 index version을 하나의 거대한 mutable 파일로 취급하지 않는다는 것이다. immutable
content와 작은 manifest를 분리하면 다운로드, 검증, rollback, 증분 갱신이 같은 구조로 해결된다.

## 권장 번들 구조

### 1. 호환성을 명시하는 manifest

`catalog-semantic.gob` 하나 대신 다음 정보를 가진 manifest를 둔다.

```json
{
  "formatVersion": 2,
  "snapshotVersion": "2026-09-01.1",
  "catalogSnapshotDigest": "sha256:...",
  "documentRecipeVersion": 1,
  "model": "embeddinggemma:300m-qat-q4_0",
  "modelDigest": "sha256:...",
  "dimensions": 768,
  "vectorEncoding": "float32-le",
  "normalization": "l2",
  "documents": 95956,
  "shards": [
    {"id": "00", "url": "...", "size": 1234567, "sha256": "..."}
  ]
}
```

`model` tag만 비교해서는 부족하다. tag가 가리키는 artifact가 바뀔 수 있으므로 immutable model digest,
차원, document/query prefix, normalization을 함께 고정해야 한다. Google은 EmbeddingGemma가 768차원뿐
아니라 512·256·128차원을 위한 Matryoshka 표현을 제공하며, truncate한 뒤 다시 정규화하도록 명시한다.
[EmbeddingGemma 공식 모델 카드](https://ai.google.dev/gemma/docs/embeddinggemma/model_card)

### 2. 문서별 재사용 key

각 entry에 대해 다음을 계산한다.

```text
documentHash = SHA256(canonicalUTF8(semanticDocument(entry)))
vectorKey = SHA256(modelDigest || dimensions || documentRecipeVersion || documentHash)
```

`SyncedAt`, 조회수, 신청수처럼 `semanticDocument`에 들어가지 않는 metadata가 바뀌어도 벡터는 그대로
재사용한다. 제목·기관·분류·설명이 바뀌거나 prompt recipe/model이 바뀐 경우만 miss가 된다.

새 catalog와 사전 구축 snapshot이 완전히 같지 않아도 전체를 거부하지 않는다.

- PK와 `documentHash`가 같은 항목: 기존 벡터 사용
- PK는 같지만 hash가 다른 항목: 재임베딩 대상
- 신규 항목: 재임베딩 대상
- 삭제 항목: 새 manifest에서 제외

Ollama가 없는 사용자는 일치하는 벡터까지만 시맨틱 검색하고 나머지는 lexical로 검색할 수 있다. 이때
`semantic.coverage = matched / catalog entries`를 출력해 부분 index 사용을 숨기지 않는다. coverage가
제품이 정한 하한보다 낮으면 안전하게 lexical-only로 폴백한다.

### 3. immutable shard와 원자적 설치

처음에는 PK hash 첫 1 byte로 256개 stable shard를 만드는 것이 단순하다. 9.6만 건이면 shard당 평균
약 375건이어서 한 entry 변경이 전체 index 재다운로드를 만들지 않는다. shard 내부는 다음처럼
연속형으로 둔다.

```text
header | sorted PK table | document hashes | packed row-major vectors
```

설치는 다음 순서를 지킨다.

1. 새 manifest를 받고 version·호환성을 검사한다.
2. 기존 cache와 SHA-256이 같은 shard는 그대로 참조한다.
3. 나머지를 `.partial` 경로에 받고 length와 SHA-256을 확인한다.
4. 완성된 version directory를 만들고 manifest를 마지막에 기록한다.
5. `CURRENT` 포인터를 원자적으로 교체한다.
6. search process가 새 snapshot을 성공적으로 열면 이전 한 세대만 남기고 나중에 정리한다.

GitHub Releases는 public release asset을 인증 없이 내려받을 수 있고 API 응답에 asset size와 SHA-256
digest를 제공한다. 개별 asset은 2GiB 미만이어야 하고 release당 1,000개까지 가능하므로 현재 규모의
첫 배포에 충분하다. [GitHub Release asset API](https://docs.github.com/en/rest/releases/assets),
[release 한도](https://docs.github.com/en/repositories/releasing-projects-on-github/about-releases)

초기에는 한 개의 압축 bundle로 first install을 단순화하고, 증분 release부터 manifest+shard로 옮길 수
있다. 프로젝트 설정에서 immutable releases를 켜면 tag와 asset 수정이 금지되고 release attestation도
생성된다. [GitHub immutable releases](https://docs.github.com/en/code-security/concepts/supply-chain-security/immutable-releases)
배포 revision이 많아져 동일 shard 재사용과 partial range download가 중요해지면 OCI artifact로 옮긴다.

## 검색 backend 선택

### 지금: packed flat mmap

9.6만 건에서는 정확한 flat cosine scan을 기준으로 유지하는 편이 맞다. 현재 검색은 한 질의가 아니라
최대 여러 discovery axis를 임베딩해 반복 scan할 수 있으므로, 우선 다음을 측정한다.

- index open 시 heap 증가량과 시간
- 1·4·8개 query vector의 warm/cold p50·p95
- 8GB RAM, 4-core CPU 환경의 동시 1·4 search
- float32 대비 float16/int8의 Recall@10, nDCG@10, 직접 관련 결과 비율

`mmap`은 파일 전체 크기만큼 heap allocation하지 않게 해주지만 모든 page access가 공짜가 되는 것은
아니다. row-major 연속 접근, query batching, top-k heap을 함께 적용해야 한다.

### 다음: 차원 축소 또는 vector 양자화

EmbeddingGemma 공식 다국어 MTEB 평균은 full precision에서 768d 61.15, 512d 60.71, 256d 59.68,
128d 58.23이다. 이는 일반 benchmark이고 한국 공공데이터 품질 보증은 아니다. 그래도 256d float16이면
순수 vector payload가 약 46.85MiB라서 가장 유력한 경량 후보이다. Ollama `/api/embed`도 `dimensions`를
요청 필드로 제공한다. [Ollama API type source](https://github.com/ollama/ollama/blob/main/api/types.go)

권장 실험 순서는 `768d float32 → 256d float32 → 256d float16 → 768d int8`이다. 차원 축소와
양자화를 동시에 적용하면 어느 변화가 품질 손실을 만들었는지 알 수 없기 때문이다.

### 나중: ANN 또는 USearch

다음 중 하나가 실측될 때만 ANN backend를 연다.

- 일반 사양에서 warm p95가 제품 latency budget을 지속적으로 넘음
- 여러 MCP client의 동시 검색으로 flat scan CPU가 병목이 됨
- corpus가 수십만~수백만 건으로 성장함

USearch는 mmap view, quantized storage, Go binding을 모두 갖춰 좋은 후보지만 native library의 플랫폼별
release와 CGO/FFI 장애면이 생긴다. 따라서 “벡터 DB 설치 없음”이라는 제품 장점을 포기하기 전에 packed
flat backend를 기준선으로 측정해야 한다.

## 사전 구축 index가 없애지 못하는 것

질의를 검색하려면 document index와 같은 embedding space의 query vector가 필요하다. Ollama는 공식적으로
indexing과 querying에 같은 embedding model을 쓰라고 안내한다.
[Ollama embeddings 문서](https://docs.ollama.com/capabilities/embeddings)

따라서 지원 모드는 명확히 나눈다.

| 모드 | 문서 index | query embedder | 결과 |
|---|---|---|---|
| 기본 | 없음 | 없음 | model-planned lexical, 즉시 사용 |
| 권장 semantic | 공식 prebuilt 다운로드 | 같은 Ollama model | 전체 hybrid 검색 |
| 부분 갱신 | prebuilt + hash 일치 벡터 | Ollama 있음 | 변경분만 로컬 임베딩 후 전체 hybrid |
| Ollama 없는 최신 catalog | 일치 snapshot 범위만 있음 | 없음 | 새 query를 임베딩할 수 없으므로 lexical 사용 |
| 개발자/커스텀 | 로컬 전체 build | 선택 provider | 사용자 책임의 완전 rebuild |

Codex·Claude·Gemini·Cursor 구독은 검색축을 계획하는 데는 사용할 수 있지만, prebuilt EmbeddingGemma
index의 query vector를 대신 만들 수 있다고 가정하면 안 된다. provider가 “embedding” 기능을 제공하는지와
무관하게 **동일한 model revision과 recipe로 같은 vector space를 재현해야 하기 때문**이다. 향후 hosted
query embedder를 추가해도 manifest의 model identity를 만족하는 명시적 provider로만 허용해야 한다.

Ollama 설치까지 없애려면 CLI에 동일 embedding model/runtime를 내장해야 한다. 이는 약 239MB model
배포, OS·architecture별 inference runtime, 모델 사용조건과 업데이트 책임을 CLI release에 편입한다.
first-run document build를 없애는 문제와 별개이므로 1차 개선 범위에는 넣지 않는다.

## 제품 도입 조건

시맨틱 검색을 사용자 필수 절차로 만들 근거는 “구현되어 있음”이 아니라 실제 검색 개선이다. 출시 전
동일한 sealed query set과 동일 snapshot에서 다음 네 경로를 비교한다.

1. 자연어 lexical
2. model-planned lexical
3. 자연어 hybrid semantic
4. model-planned hybrid semantic

필수 보고 항목은 Recall@10, nDCG@10, 직접 관련 결과 비율, lexical로는 0건이지만 semantic이 찾은 유효
데이터 수, 심각한 semantic noise 수다. 여기에 설치·운영 지표를 함께 둔다.

- 일반 사양에서 prebuilt first install 시간과 peak RAM
- snapshot 갱신 시 재사용률, 다운로드 bytes, 재임베딩 건수
- warm/cold 검색 p50·p95
- index coverage와 검증 실패/rollback 여부

권장 정책은 **품질 이득이 유의미하더라도 설치는 자동 prebuilt download로 줄이고, Ollama는 계속
선택사항으로 두는 것**이다. 시맨틱이 명확히 더 좋은 broad-intent 질문에서는 설치 안내를 보여주되,
정확한 API명이나 고유 데이터셋을 찾는 사용자를 강제로 거치게 하지 않는다.

## 구현 순서

1. `SemanticIndex v2`: 문서 hash, model digest, dimensions, recipe version을 도입한다.
2. 기존 index에서 hash가 같은 벡터를 재사용하는 incremental builder를 먼저 만든다.
3. packed flat file + mmap reader를 추가하고 기존 float32 flat 결과와 일치하는지 검증한다.
4. CI 또는 release job이 catalog와 default semantic bundle을 함께 생성한다.
5. `semantic-update`가 manifest download, SHA-256 검증, atomic activate, rollback을 수행하게 한다.
6. 일반 사양 benchmark와 sealed-query 품질 평가를 README에 공개한다.
7. 256d/float16/int8 중 품질 gate를 통과한 경량 variant만 추가한다.
8. flat scan이 실제 병목으로 확인될 때 USearch/ANN backend를 별도 ADR로 검토한다.

이 순서라면 사용자는 처음부터 95,956건을 임베딩하지 않고, 유지보수자는 전체 index를 매번 다시 만들지
않으며, oddsock은 별도 vector DB를 필수 설치하지 않는 현재 장점을 유지할 수 있다.
