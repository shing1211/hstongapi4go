# hstongapi4go

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go" alt="Go">
  <img src="https://img.shields.io/badge/License-Apache%202.0-blue?style=flat-square" alt="라이선스">
  <img src="https://img.shields.io/badge/Status-alpha-blue?style=flat-square" alt="상태">
  <img src="https://img.shields.io/badge/Gateway-v2.4.1-brightgreen?style=flat-square" alt="Gateway 버전">
  <img src="https://img.shields.io/badge/Protobuf-v2.2.0-blueviolet?style=flat-square" alt="Protobuf 패키지 버전">
  <img src="https://img.shields.io/badge/Endpoints-51-orange?style=flat-square" alt="HTTP 엔드포인트">
  <img src="https://img.shields.io/badge/Push%20topics-11-yellowgreen?style=flat-square" alt="시세 푸시 토픽">
  <a href="https://shing1211.github.io/hstongapi4go/"><img src="https://img.shields.io/badge/Docs-GitHub%20Pages-97CAFF?style=flat-square&logo=github" alt="문서"></a>
</p>

> **비공식 & 알파.** `hstongapi4go`는 HStong(華盛) Quant OpenAPI **로컬 Gateway**를
> 위한 커뮤니티 Go SDK입니다. HStong / 華盛 및 그 자회사와 **제휴·승인·보증·후원
> 관계가 전혀 없습니다**. "HStong", "華盛", "華盛通" 및 관련 명칭과 상표는 각 소유자의
> 자산이며 여기서는 설명 및 상호운용 목적으로만 사용됩니다.
> [DISCLAIMER.md](./DISCLAIMER.md)를 참고하세요.

> **Go 네이티브 · 타입 안전 · Gateway 우선.** 로컬 HStong Gateway를 위한 관용적인 Go
> 클라이언트 — 시세 데이터, 트레이딩, 선물, 알고, 실시간 푸시 — 금액과 수량은 항상
> 문자열로 유지하고 주문 변경은 자동으로 재시도하지 않습니다.

[English](./README.md) · [简体中文](./README.zh-Hans.md) · [繁體中文](./README.zh-Hant.md) · [日本語](./README.ja.md) · [한국어](./README.ko.md) · [Español](./README.es.md)

> 이 문서는 영어 [README](./README.md)의 커뮤니티 번역입니다. **영어판이 정본입니다.**
> 동기화 / Last synced: 2026-09-30

## 목차

- [상태](#상태)
- [설치](#설치)
- [빠른 시작 — 라이브 거래](#빠른-시작--라이브-거래)
- [빠른 시작 — 오프라인 / 모의](#빠른-시작--오프라인--모의)
- [v1.0으로 마이그레이션](#v10으로-마이그레이션)
- [기능 매트릭스](#기능-매트릭스)
- [구성](#구성)
- [패키지 구조](#패키지-구조)
- [문서](#문서)
- [빌드 및 테스트](#빌드-및-테스트)
- [기여](#기여)
- [보안](#보안)
- [라이선스](#라이선스)

---

## 상태

| 항목 | 상태 |
|------|------|
| 프로토콜 코어(HTTP 엔벨로프, 라우트 별칭, 하이브리드 코덱, 타입 지정 오류) | 구현됨 |
| 시세 데이터 조회(9개 엔드포인트) | 구현됨 |
| 시세 구독 + TCP 푸시(11개 토픽) | 구현됨 |
| 트레이딩 세션(로그인/로그아웃, 킵얼라이브, 단일 비행 재로그인) | 구현됨 |
| 트레이딩 자산 / 포지션(5) 및 주문(13) | 구현됨 |
| 트레이딩 푸시 구독 + 주문 상태 디코더 | 구현됨 |
| 선물(11개 엔드포인트 + 푸시) | 구현됨 |
| 알고 / 전략(7개 엔드포인트) | 구현됨 |
| 채널 기반 스트리밍 API(`Updates()` / `Errors()`) | 구현됨 |
| 모의 Gateway(51개 HTTP 라우트 + TCP 푸시) 및 독립 실행 파일 | 구현됨 |
| 강화(레이트 리밋, 재시도 예산, 서킷 브레이커, 로깅, 메트릭) | 구현됨 |
| 문서(README, MkDocs 사이트, ADR, SPEC, LEGACY) | 구현됨 |
| 오프라인 테스트 + 전 엔드포인트 SDK 대 모의 e2e | 구현됨 |
| 실제 Gateway 대상 통합 테스트 | 작성됨·환경 변수로 게이트됨. 실사용 확인은 사용자 실행 대기 |
| 릴리스(GitHub,工件는 Gitee에 배포되지 않음을 결정으로 결정) | v1.0.3 |

51개 HTTP 엔드포인트와 11개 시세 푸시 토픽이 모두 구현되었습니다. 수치는
[docs/SPEC.md](./docs/SPEC.md)가 정본입니다. 다른 곳에서 수동으로 편집하지 마세요.

## 설치

```bash
go get github.com/shing1211/hstongapi4go
```

**Go 1.26+** 와 실행 중인 로컬 [HStong OpenAPI Gateway](https://quant-open.hstong.com/api-docs/)
(또는 저장소 내 [Mock Gateway](./docs/mock-gateway.md))가 필요합니다. SDK는 Gateway를
설치·시작·재배포하지 않습니다. 기본적으로 `http://127.0.0.1:11111`(HTTP)과
`127.0.0.1:11112`(TCP 푸시)에 연결합니다.

## 빠른 시작 — 라이브 거래

1. **[API 액세스 신청](docs/getting-started-live.md)** —华盛 계정, RSA 공개 키 업로드 필요,
   승인까지 2–3영업일.
2. **[华盛 Gateway 설치](docs/getting-started-live.md)** — quant-open.hstong.com에서 다운로드,
   로컬에서 실행.
3. **[통합 테스트로 검증](docs/integration-testing.md)** — 실제 주문 전에 전체 스택 검증.
4. **[첫 번째 주문 실행](examples/trading)**：

   ```sh
   HSTONG_TRADE_PASSWORD=yourpassword \
   HSTONG_EXAMPLE_PLACE_ORDER=1 \
     go run ./examples/trading
   ```

   `HSTONG_EXAMPLE_PLACE_ORDER` 플래그는 명시적 opt-in으로, 환경 변수 잊어버려서
  误주문를 방지합니다.

## 빠른 시작 — 오프라인 / 모의

 réelle 계정 없이 SDK를 탐색하려면 내장 모의 Gateway를 사용：

```sh
go run ./cmd/hstong-mock-gateway &   # :11111/:11112에서 실행
go run ./examples/quickstart          # 자격 증명 불필요
```

모든 표면에 대해 실행 가능하고 자격 증명 없이 컴파일되는 프로그램도
[`examples/`](./examples/README.md)에 있습니다: `quickstart`, `market-data`, `trading`,
`futures`, `algo`, `streaming`.

## v1.0으로 마이그레이션

v1.0.0은 지금 사용 중인 API를 **변경하지 않습니다**. `pkg/hstong/*`, `pkg/types/`,
`client/` 는 그대로이고 기본값으로 남습니다. 따로 할 일은 없습니다.
v1.0.0이 추가하는 것은 동일한 51개 엔드포인트를 계층화하고 완성된 두 번째
구현이며, 필요할 때 선택해 사용합니다.

```go
// 여전히 기본값이며, 여전히 올바르게 동작합니다:
m := market.New(c)

// v-next 계층을 선택합니다:
stack, err := services.NewStack(c)
if err != nil {
    return err
}
defer stack.Close()
q, err := stack.Market.BasicQot(ctx, req)
```

마이그레이션은 호출 단위로 점진적입니다. `pkg/hstong` 호출을 `pkg/services` 호출로
자신 속도에 맞춰 바꾸면 되고, 둘은 공존합니다. 컴파일되는 샘플과 함께 있는 완전한
가이드는 [docs/MIGRATION.md](./docs/MIGRATION.md)에 있습니다.

시작하기 전에 알아둘 두 가지:

- **주문 변경은 자동 재시도하지 않습니다.** 두 인터페이스 모두 마찬가지입니다.
  trade·futures·algo 변경은 정확히 한 번만 시도하며, 모호한 실패는 재시도한다는
  뜻이 아니라 호출자가 대조한다는 뜻입니다. 이는
  [ADR 0003](./docs/adr/0003-no-auto-retry-orders.md)이며 마이그레이션해도
  바뀌지 않습니다.
- **금액과 수량은 `string` / `json.Number`이며 `float64`이 아닙니다.**

절차보다 근거를 알고 싶다면 `docs/VNEXT.md`에 무엇을 만들었고 무엇을 의도적으로 남겨 두었는지 기록되어 있습니다. 그리고 [CHANGELOG.md](./CHANGELOG.md)의 `v1.0.0` 항목에는 이번 릴리스가 **주장하지 않는** 것이 나열되어 있으며, 실제 Gateway에 대해 한 번도 실행되지 않았다는 점도 포함됩니다.


## 기능 매트릭스

엔드포인트 수는 [docs/SPEC.md](./docs/SPEC.md)에서만 가져옵니다. 모든 라우트는
`POST http://127.0.0.1:11111<route>`입니다.

| 표면 | 패키지 | 엔드포인트 수 |
|---------|---------|-----------|
| 시세 데이터 조회 | `pkg/hstong/market` | 9 |
| 시세 구독 / 해제 | `pkg/hstong/market` | 2 |
| 트레이딩 세션 | `pkg/hstong` | 2 |
| 트레이딩 자산 / 포지션 | `pkg/hstong/trade` | 5 |
| 트레이딩 주문 | `pkg/hstong/trade` | 13 |
| 트레이딩 푸시 구독 | `pkg/hstong/trade` | 2 |
| 알고 / 전략 | `pkg/hstong/algo` | 7 |
| 선물 | `pkg/hstong/future` | 11 |
| **HTTP 합계** | | **51** |

시세 푸시 토픽(11): `11`, `35`(호가); `14`, `27`, `28`, `37`(틱);
`16`(브로커 큐); `17`, `25`, `26`, `36`(호가창). [docs/SPEC.md §3](./docs/SPEC.md)을
참고하세요. 트레이딩과 선물의 주문 상태 푸시는 동일한 TCP 채널의
`TradeStockDeliverNotify` 계열을 사용합니다.

하이브리드 코덱은 **엔드포인트별**입니다. 시세 HTTP 본문은 타입 지정 DTO를
`encoding/json`으로 디코딩하고, TCP 푸시 페이로드는 바이너리 protobuf로 처리합니다.
트레이딩·선물·알고·자산·세션 본문은 수작업 JSON 구조체입니다. 금액과 수량은 `string`
(또는 `json.Number`)이며 `float64`가 아닙니다. 주문, 선물, 알고 변경은 정확히 한 번만
시도하며 자동 재시도하지 않습니다.

## 구성

`client.New`는 문서화된 기본값 위에 함수형 옵션을 순서대로 적용합니다. 아래 모든
`HSTONG_*` 변수는 `client.WithEnv()`가 읽습니다. `WithEnv` 뒤에 둔 명시적 옵션은 환경
변수를 재정의합니다.

| 변수 | 형식 | 기본값 | 용도 |
|----------|--------|---------|---------|
| `HSTONG_GATEWAY_URL` | 절대 `http`/`https` URL | `http://127.0.0.1:11111` | Gateway HTTP 루트 |
| `HSTONG_PUSH_ADDR` | `host:port` | `127.0.0.1:11112` | Gateway TCP 푸시 주소 |
| `HSTONG_TIMEOUT` | Go duration(`10s`, `1500ms`) | `10s` | 요청별 타임아웃과 엔벨로프 `timeout_sec` |
| `HSTONG_TRADE_PASSWORD` | 평문 | 미설정 | 트레이딩 비밀번호. `TradeLogin` 전에 암호화하며 로그에 남기지 않음 |
| `HSTONG_VERIFY_PUSH` | `strconv.ParseBool` | `false` | 선택적으로 푸시 프레임 `SHA1WithRSA` 검증 활성화 |

실제 Gateway 대상 통합 테스트는 기본적으로 건너뛰며 자체 변수를 사용합니다
([`test/integration/README.md`](./test/integration/README.md) 참고):

| 변수 | 필수 | 기본값 | 용도 |
|----------|----------|---------|---------|
| `HSTONG_INTEGRATION` | 예(게이트) | — | `1`이 아니면 모든 테스트가 건너뛰어짐 |
| `HSTONG_GATEWAY_URL` | 아니요 | `http://127.0.0.1:11111` | Gateway HTTP 루트 |
| `HSTONG_PUSH_ADDR` | 아니요 | `127.0.0.1:11112` | Gateway TCP 푸시 주소 |
| `HSTONG_TRADE_PASSWORD` | 세션/트레이딩 테스트에 필요 | — | 평문 트레이딩 비밀번호 |
| `HSTONG_TEST_SYMBOL` | 아니요 | `00700.HK` | 테스트 대상 종목 |
| `HSTONG_TEST_EXCHANGE` | 아니요 | `K` | `K` 홍콩, `P` 미국, `v` 선전, `t` 상하이 |
| `HSTONG_VERIFY_PUSH` | 아니요 | 꺼짐 | 푸시 서명 검증 활성화 |
| `HSTONG_PLACE_ORDERS` | 아니요 | 꺼짐 | 주문 변경 테스트를 켜려면 `1`이어야 함 |

## 패키지 구조

```
hstongapi4go/
├── client/            # 코어 클라이언트: 옵션, 환경 변수, 라우트, 코덱, Close
├── pkg/hstong/        # 공개 관리자: 세션 + market/trade/future/algo/stream
├── pkg/types/         # 열거형, 상태 코드, 플랫폼 공개 키
├── pkg/domain/        # v-next: decimal value types, typed IDs, DTO mappers
├── pkg/services/      # v-next: Stack + market/account/trading/algo/futures use cases
├── pkg/transport/     # v-next: wire mappers, pagination encoding, PushAdapter
├── internal/          # 내부: transport, push, crypto, errs, resilience, logging, metrics
├── gen/               # 생성된 protobuf 코드(편집 금지)
├── proto/             # 동봉 .proto 소스 + 출처
├── test/mockgateway/  # 오프라인 mock HTTP + TCP 푸시 서버
├── test/e2e/          # 전 엔드포인트 SDK 대 모의 테스트
├── test/integration/  # 환경 변수로 게이트된 실제 Gateway 테스트(기본 건너뜀)
├── cmd/               # 독립 실행 파일(hstong-mock-gateway)
├── examples/          # 실행 가능한 예제(mock 친화)
├── scripts/           # 빌드 및 검증 스크립트
└── docs/              # MkDocs 사이트, SPEC, ADR, DESIGN, LEGACY
```

## 문서

- 문서 사이트: <https://shing1211.github.io/hstongapi4go/>
- 정본 API 색인과 수치: [docs/SPEC.md](./docs/SPEC.md)
- 아키텍처: [ARCHITECTURE.md](./ARCHITECTURE.md) · [docs/DESIGN.md](./docs/DESIGN.md)
- 결정: [docs/adr/README.md](./docs/adr/README.md)(ADR 0001–0011)
- 레거시 프로토콜(문서화만 됨, **미구현**): [docs/LEGACY.md](./docs/LEGACY.md)
- 면책 조항: [DISCLAIMER.md](./DISCLAIMER.md)

**레거시 플랫폼 직결 프로토콜** —— `/hs/v2/login`, `/hs/config/queryServer`, 개발자
RSA 서명을 사용하는 직접 151바이트 socket, AES-ECB 동적 키, 하트비트, 디바이스
바인딩 —— 은 참고용으로만 문서화되어 있으며 본 SDK에서 **구현되지 않았습니다**.
[docs/LEGACY.md](./docs/LEGACY.md)를 참고하세요.

## 빌드 및 테스트

```bash
make help            # 타깃 목록
make build           # go build ./...
make check           # fmt + vet + money-check + 단위 테스트
make test-race       # go test -race -count=1 ./...
make test-integration  # 환경 변수로 게이트된 실제 Gateway 테스트(HSTONG_* 필요)
make proto-verify    # gen/이 proto/에서 벗어나면 실패
make docs-check      # markdown 링크 검사 + mkdocs build --strict
make mock-gateway    # 독립 모의 Gateway 실행
```

자격 증명 없이 통과해야 하는 직접 명령:

```bash
go build ./...
go vet ./...
gofmt -l .            # 아무것도 출력하지 않아야 함
go test ./...
go test -race -count=1 ./...
```

단위 테스트는 오프라인이며 자격 증명이 필요 없습니다. 통합 테스트는 네트워크나 자격
증명이 필요할 수 있는 유일한 테스트입니다.

## 기여

[CONTRIBUTING.md](./CONTRIBUTING.md)를 참고하세요. 모든 커밋은 DCO 서명
(`git commit -s`)이 필요하며 run과 task ID를 참조하는 Conventional Commits를 따릅니다.
새 런타임 의존성에는 ADR이 필요합니다. `gen/` 아래의 생성 코드를 편집하지 마세요. 주문
변경을 자동 재시도하지 마세요.

## 보안

[SECURITY.md](./SECURITY.md)를 참고하세요. SDK는 플랫폼 자격 증명도, 개발자 개인 키도
보유하지 않습니다. 동봉된 플랫폼 공개 키는 공개 참조 데이터입니다. SDK가 다루는 유일한
민감 값은 평문 트레이딩 비밀번호이며, 이는 전송 중에 암호화하고 로그에 남기지 않습니다.

## 라이선스

[Apache License 2.0](./LICENSE). [NOTICE](./NOTICE),
[DISCLAIMER.md](./DISCLAIMER.md), [THIRD_PARTY_NOTICES.md](./THIRD_PARTY_NOTICES.md)도
참고하세요.
