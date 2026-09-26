# hstongapi4go

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go" alt="Go">
  <img src="https://img.shields.io/badge/License-Apache%202.0-blue?style=flat-square" alt="授權條款">
  <img src="https://img.shields.io/badge/Status-alpha-blue?style=flat-square" alt="狀態">
  <img src="https://img.shields.io/badge/Gateway-v2.4.1-brightgreen?style=flat-square" alt="Gateway 版本">
  <img src="https://img.shields.io/badge/Protobuf-v2.2.0-blueviolet?style=flat-square" alt="Protobuf 套件版本">
  <img src="https://img.shields.io/badge/Endpoints-51-orange?style=flat-square" alt="HTTP 端點">
  <img src="https://img.shields.io/badge/Push%20topics-11-yellowgreen?style=flat-square" alt="行情推送主題">
  <a href="https://shing1211.github.io/hstongapi4go/"><img src="https://img.shields.io/badge/Docs-GitHub%20Pages-97CAFF?style=flat-square&logo=github" alt="文件"></a>
</p>

> **非官方 & 早期預覽版。** `hstongapi4go` 是面向 HStong（華盛）Quant OpenAPI
> **本機 Gateway** 的社群 Go SDK。它**與** HStong / 華盛及其任何子公司**無隸屬、
> 授權、背書或贊助關係**。"HStong"、"華盛"、"華盛通"及相關名稱和商標歸各自所有者
> 所有，此處僅用於描述性及互通性目的。參見 [DISCLAIMER.md](./DISCLAIMER.md)。

> **Go 原生 · 型別安全 · Gateway 優先。** 面向本機 HStong Gateway 的慣用 Go 用戶端
> —— 行情資料、交易、期貨、演算法與即時推送 —— 金額和數量一律以字串保存，訂單變更
> 絕不自動重試。

[English](./README.md) · [简体中文](./README.zh-Hans.md) · [繁體中文](./README.zh-Hant.md) · [日本語](./README.ja.md) · [한국어](./README.ko.md) · [Español](./README.es.md)

> 本文件是英文 [README](./README.md) 的社群翻譯。**英文版本為準。**
> 同步於 / Last synced: 2026-09-25

## 目錄

- [狀態](#狀態)
- [安裝](#安裝)
- [快速開始](#快速開始)
- [功能矩陣](#功能矩陣)
- [設定](#設定)
- [套件結構](#套件結構)
- [文件](#文件)
- [建置與測試](#建置與測試)
- [貢獻](#貢獻)
- [安全性](#安全性)
- [授權條款](#授權條款)

---

## 狀態

| 項目 | 狀態 |
|------|------|
| 協定核心（HTTP 信封、路由別名、混合編解碼器、具型別錯誤） | 已實作 |
| 行情資料拉取（9 個端點） | 已實作 |
| 行情訂閱 + TCP 推送（11 個主題） | 已實作 |
| 交易工作階段（登入/登出、保活、單飛重新登入） | 已實作 |
| 交易資產 / 持倉（5）與訂單（13） | 已實作 |
| 交易推送訂閱 + 訂單狀態解碼器 | 已實作 |
| 期貨（11 個端點 + 推送） | 已實作 |
| 演算法 / 策略（7 個端點） | 已實作 |
| 以 channel 為基礎的串流 API（`Updates()` / `Errors()`） | 已實作 |
| Mock Gateway（51 條 HTTP 路由 + TCP 推送）與獨立二進位 | 已實作 |
| 加固（限流、重試預算、斷路器、日誌、指標） | 已實作 |
| 文件（README、MkDocs 網站、ADR、SPEC、LEGACY） | 已實作 |
| 離線測試 + 全端點 SDK 對 mock 的端對端測試 | 已實作 |
| 針對真實 Gateway 的整合測試 | 已撰寫並以環境變數閘控；待使用者實際執行確認 |
| 發佈（GitHub + Gitee） | v0.1.13 ✓ |

全部 51 個 HTTP 端點和 11 個行情推送主題均已實作。計數以
[docs/SPEC.md](./docs/SPEC.md) 為準；請勿在其他地方手動修改。

## 安裝

```bash
go get github.com/shing1211/hstongapi4go
```

需要 **Go 1.26+** 以及正在執行的本機 [HStong OpenAPI Gateway](https://quant-open.hstong.com/api-docs/)
（或倉庫內的 [Mock Gateway](./docs/mock-gateway.md)）。SDK 從不安裝、啟動或再分發
Gateway；預設連線 `http://127.0.0.1:11111`（HTTP）和 `127.0.0.1:11112`（TCP 推送）。

## 快速開始

建置一次用戶端，共用使用，並在程式結束時關閉。用戶端可安全並行使用。

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	"github.com/shing1211/hstongapi4go/pkg/hstong"
	"github.com/shing1211/hstongapi4go/pkg/hstong/market"
	"github.com/shing1211/hstongapi4go/pkg/hstong/trade"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. 從 HSTONG_* 解析設定（見「設定」章節）。
	c, err := client.New(client.WithEnv())
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	// 2. 登入交易工作階段（需要 HSTONG_TRADE_PASSWORD）。
	session := hstong.NewSessionManager(c)
	if err := session.Login(ctx); err != nil {
		log.Fatalf("trade login: %v", err)
	}
	defer session.Logout(ctx)

	// 3. 請求一筆行情報價。
	marketMgr := market.New(c)
	quote, err := marketMgr.BasicQot(ctx, market.BasicQotRequest{
		Security: []*dto.Security{
			{DataType: int32(types.DataTypeHKStock), Code: "0700.HK"},
		},
		MktTmType: 1,
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, q := range quote.BasicQot {
		fmt.Printf("quote %s last=%v\n", q.GetSecurity().GetCode(), q.GetLastPrice())
	}

	// 4. 查詢今日真實訂單（需認證）。
	tradeMgr := trade.New(c, trade.WithSession(session))
	orders, err := tradeMgr.RealEntrustList(ctx, trade.RealEntrustListRequest{
		ExchangeType: types.ExchangeHK,
		QueryCount:   20,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("today's real orders: %d\n", len(orders))
}
```

透過 TCP 通道訂閱行情推送主題（匯入
`github.com/shing1211/hstongapi4go/pkg/hstong/stream`）：

```go
s := stream.New(c)
if err := s.Connect(ctx); err != nil {
	log.Fatal(err)
}
defer s.Close()

sub, err := s.Subscribe(ctx, types.TopicBasicQot,
	&dto.Security{DataType: int32(types.DataTypeHKStock), Code: "0700.HK"})
if err != nil {
	log.Fatal(err)
}
defer sub.Cancel(context.Background())

for {
	select {
	case <-ctx.Done():
		return
	case ev := <-sub.Updates():
		if q, ok := ev.BasicQot(); ok {
			fmt.Printf("%s last=%v\n", ev.ID, q.GetBasicQot().GetLastPrice())
		}
	case err := <-sub.Errors():
		log.Printf("stream: %v", err)
	}
}
```

[`examples/`](./examples/README.md) 中提供涵蓋每個介面的、可執行且無需憑證即可編譯的程式：
`quickstart`、`market-data`、`trading`、`futures`、`algo` 和 `streaming`。

## 功能矩陣

端點計數僅取自 [docs/SPEC.md](./docs/SPEC.md)。所有路由均為
`POST http://127.0.0.1:11111<route>`。

| 介面 | 套件 | 端點數 |
|------|-----|--------|
| 行情資料拉取 | `pkg/hstong/market` | 9 |
| 行情訂閱 / 取消訂閱 | `pkg/hstong/market` | 2 |
| 交易工作階段 | `pkg/hstong` | 2 |
| 交易資產 / 持倉 | `pkg/hstong/trade` | 5 |
| 交易訂單 | `pkg/hstong/trade` | 13 |
| 交易推送訂閱 | `pkg/hstong/trade` | 2 |
| 演算法 / 策略 | `pkg/hstong/algo` | 7 |
| 期貨 | `pkg/hstong/future` | 11 |
| **HTTP 合計** | | **51** |

行情推送主題（11）：`11`、`35`（報價）；`14`、`27`、`28`、`37`（逐筆）；
`16`（經紀隊列）；`17`、`25`、`26`、`36`（訂單簿）。參見
[docs/SPEC.md §3](./docs/SPEC.md)。交易與期貨的訂單狀態推送使用同一 TCP 通道上的
`TradeStockDeliverNotify` 系列。

混合編解碼器是**按端點**的：行情 HTTP 回應主體以 `encoding/json` 解碼為具型別 DTO，
TCP 推送酬載使用二進位 protobuf；交易、期貨、演算法、資產和工作階段主體為手寫 JSON
結構。金額和數量為 `string`（或 `json.Number`），絕不是 `float64`。訂單、期貨和
演算法變更只發出一次嘗試，絕不自動重試。

## 設定

`client.New` 依序在文件化預設值之上套用函式選項。下面每個 `HSTONG_*` 變數都由
`client.WithEnv()` 讀取；放在 `WithEnv` 之後的明確選項會覆寫環境變數。

| 變數 | 格式 | 預設值 | 用途 |
|----------|--------|---------|---------|
| `HSTONG_GATEWAY_URL` | 絕對 `http`/`https` URL | `http://127.0.0.1:11111` | Gateway HTTP 根位址 |
| `HSTONG_PUSH_ADDR` | `host:port` | `127.0.0.1:11112` | Gateway TCP 推送位址 |
| `HSTONG_TIMEOUT` | Go duration（`10s`、`1500ms`） | `10s` | 單一請求逾時與信封 `timeout_sec` |
| `HSTONG_TRADE_PASSWORD` | 明文 | 未設定 | 交易密碼；在 `TradeLogin` 前加密，絕不記錄日誌 |
| `HSTONG_VERIFY_PUSH` | `strconv.ParseBool` | `false` | 可選開啟推送訊框 `SHA1WithRSA` 驗簽 |

針對真實 Gateway 的整合測試預設跳過，並有自己的變數（參見
[`test/integration/README.md`](./test/integration/README.md)）：

| 變數 | 是否必需 | 預設值 | 用途 |
|----------|----------|---------|---------|
| `HSTONG_INTEGRATION` | 是（閘控） | — | 必須為 `1`，否則所有測試跳過 |
| `HSTONG_GATEWAY_URL` | 否 | `http://127.0.0.1:11111` | Gateway HTTP 根位址 |
| `HSTONG_PUSH_ADDR` | 否 | `127.0.0.1:11112` | Gateway TCP 推送位址 |
| `HSTONG_TRADE_PASSWORD` | 工作階段/交易測試需要 | — | 明文交易密碼 |
| `HSTONG_TEST_SYMBOL` | 否 | `00700.HK` | 被測標的 |
| `HSTONG_TEST_EXCHANGE` | 否 | `K` | `K` 港股、`P` 美股、`v` 深市、`t` 滬市 |
| `HSTONG_VERIFY_PUSH` | 否 | 關閉 | 啟用推送簽章驗證 |
| `HSTONG_PLACE_ORDERS` | 否 | 關閉 | 必須為 `1` 才啟用訂單變更測試 |

## 套件結構

```
hstongapi4go/
├── client/            # 核心用戶端：選項、環境變數、路由、編解碼器、Close
├── pkg/hstong/        # 公開管理器：工作階段 + market/trade/future/algo/stream
├── pkg/types/         # 列舉、狀態碼、平台公鑰
├── pkg/domain/        # v-next: decimal value types, typed IDs, DTO mappers
├── pkg/services/      # v-next: market/account/trading use cases
├── pkg/transport/     # v-next: HTTP adapter (deadlines, caps, retry)
├── internal/          # 私有：transport、push、crypto、errs、resilience、logging、metrics
├── gen/               # 產生的 protobuf 程式碼（請勿編輯）
├── proto/             # 內建 .proto 來源 + 來源說明
├── test/mockgateway/  # 離線 mock HTTP + TCP 推送伺服器
├── test/e2e/          # 全端點 SDK 對 mock 測試
├── test/integration/  # 以環境變數閘控的真實 Gateway 測試（預設跳過）
├── cmd/               # 獨立二進位（hstong-mock-gateway）
├── examples/          # 可執行範例（適配 mock）
├── scripts/           # 建置與驗證腳本
└── docs/              # MkDocs 網站、SPEC、ADR、DESIGN、LEGACY
```

## 文件

- 文件網站：<https://shing1211.github.io/hstongapi4go/>
- 規範 API 索引與計數：[docs/SPEC.md](./docs/SPEC.md)
- 架構：[ARCHITECTURE.md](./ARCHITECTURE.md) · [docs/DESIGN.md](./docs/DESIGN.md)
- 決策：[docs/adr/README.md](./docs/adr/README.md)（ADR 0001–0011）
- 舊版協定（已文件化，**未實作**）：[docs/LEGACY.md](./docs/LEGACY.md)
- 免責聲明：[DISCLAIMER.md](./DISCLAIMER.md)

**舊版直連平台協定** —— `/hs/v2/login`、`/hs/config/queryServer`、使用開發者 RSA
簽名的直連 151 位元組 socket、AES-ECB 動態金鑰、心跳與裝置綁定 —— 僅為參考而文件化，
本 SDK **未實作**。參見 [docs/LEGACY.md](./docs/LEGACY.md)。

## 建置與測試

```bash
make help            # 列出目標
make build           # go build ./...
make check           # fmt + vet + money-check + 單元測試
make test-race       # go test -race -count=1 ./...
make test-integration  # 以環境變數閘控的真實 Gateway 測試（需要 HSTONG_*）
make proto-verify    # 若 gen/ 與 proto/ 漂移則失敗
make docs-check      # markdown 連結檢查 + mkdocs build --strict
make mock-gateway    # 執行獨立的 mock Gateway
```

無需憑證即可通過的直接命令：

```bash
go build ./...
go vet ./...
gofmt -l .            # 必須無輸出
go test ./...
go test -race -count=1 ./...
```

單元測試離線且無需憑證。整合測試是唯一可能需要網路或憑證的測試。

## 貢獻

參見 [CONTRIBUTING.md](./CONTRIBUTING.md)。所有提交必須 DCO 簽署
（`git commit -s`）並遵循引用 run 和 task ID 的 Conventional Commits。新的執行階段
相依性需要 ADR；絕不編輯 `gen/` 下的產生程式碼；絕不對訂單變更自動重試。

## 安全性

參見 [SECURITY.md](./SECURITY.md)。SDK 不持有平台憑證，也不持有開發者私鑰；內建的
平台公鑰為公開參考資料。SDK 處理的唯一敏感值是明文交易密碼，它在傳輸中加密且絕不
記錄日誌。

## 授權條款

[Apache License 2.0](./LICENSE)。參見 [NOTICE](./NOTICE)、
[DISCLAIMER.md](./DISCLAIMER.md) 和 [THIRD_PARTY_NOTICES.md](./THIRD_PARTY_NOTICES.md)。
