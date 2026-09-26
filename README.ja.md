# hstongapi4go

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go" alt="Go">
  <img src="https://img.shields.io/badge/License-Apache%202.0-blue?style=flat-square" alt="ライセンス">
  <img src="https://img.shields.io/badge/Status-alpha-blue?style=flat-square" alt="ステータス">
  <img src="https://img.shields.io/badge/Gateway-v2.4.1-brightgreen?style=flat-square" alt="Gateway バージョン">
  <img src="https://img.shields.io/badge/Protobuf-v2.2.0-blueviolet?style=flat-square" alt="Protobuf パッケージバージョン">
  <img src="https://img.shields.io/badge/Endpoints-51-orange?style=flat-square" alt="HTTP エンドポイント">
  <img src="https://img.shields.io/badge/Push%20topics-11-yellowgreen?style=flat-square" alt="相場配信トピック">
  <a href="https://shing1211.github.io/hstongapi4go/"><img src="https://img.shields.io/badge/Docs-GitHub%20Pages-97CAFF?style=flat-square&logo=github" alt="ドキュメント"></a>
</p>

> **非公式 & アルファ版。** `hstongapi4go` は HStong（華盛）Quant OpenAPI の
> **ローカル Gateway** 向けコミュニティ Go SDK です。HStong / 華盛およびその子会社とは
> **提携・承認・推奨・スポンサーのいずれの関係もありません**。"HStong"、"華盛"、
> "華盛通" および関連する名称・商標は各所有者に帰属し、ここでは記述的・相互運用目的
> でのみ使用しています。[DISCLAIMER.md](./DISCLAIMER.md) をご覧ください。

> **Go ネイティブ · 型安全 · Gateway ファースト。** ローカル HStong Gateway 向けの
> 慣用的な Go クライアント — マーケットデータ、取引、先物、アルゴ、リアルタイム配信 —
> 金額と数量は常に文字列で保持し、注文変更は自動再試行しません。

[English](./README.md) · [简体中文](./README.zh-Hans.md) · [繁體中文](./README.zh-Hant.md) · [日本語](./README.ja.md) · [한국어](./README.ko.md) · [Español](./README.es.md)

> 本書は英語版 [README](./README.md) のコミュニティ翻訳です。**英語版が正式です。**
> 同期 / Last synced: 2026-09-25

## 目次

- [ステータス](#ステータス)
- [インストール](#インストール)
- [クイックスタート](#クイックスタート)
- [機能マトリクス](#機能マトリクス)
- [設定](#設定)
- [パッケージ構成](#パッケージ構成)
- [ドキュメント](#ドキュメント)
- [ビルドとテスト](#ビルドとテスト)
- [コントリビュート](#コントリビュート)
- [セキュリティ](#セキュリティ)
- [ライセンス](#ライセンス)

---

## ステータス

| 項目 | 状態 |
|------|------|
| プロトコルコア（HTTP エンベロープ、ルートエイリアス、ハイブリッドコーデック、型付きエラー） | 実装済み |
| マーケットデータ取得（9 エンドポイント） | 実装済み |
| 相場購読 + TCP 配信（11 トピック） | 実装済み |
| 取引セッション（ログイン/ログアウト、キープアライブ、シングルフライト再ログイン） | 実装済み |
| 取引資産 / ポジション（5）と注文（13） | 実装済み |
| 取引配信購読 + 注文ステータスデコーダ | 実装済み |
| 先物（11 エンドポイント + 配信） | 実装済み |
| アルゴ / ストラテジー（7 エンドポイント） | 実装済み |
| チャネルベースのストリーミング API（`Updates()` / `Errors()`） | 実装済み |
| モック Gateway（51 HTTP ルート + TCP 配信）と単体バイナリ | 実装済み |
| 堅牢化（レート制限、リトライ予算、サーキットブレーカー、ロギング、メトリクス） | 実装済み |
| ドキュメント（README、MkDocs サイト、ADR、SPEC、LEGACY） | 実装済み |
| オフラインテスト + 全エンドポイントの SDK 対モック e2e | 実装済み |
| 実 Gateway に対する統合テスト | 作成済み・環境変数でゲート。実機確認はユーザー実行待ち |
| リリース（GitHub + Gitee） | v0.1.14 ✓ |

51 の HTTP エンドポイントと 11 の相場配信トピックがすべて実装済みです。件数は
[docs/SPEC.md](./docs/SPEC.md) が正式です。他の場所で手動編集しないでください。

## インストール

```bash
go get github.com/shing1211/hstongapi4go
```

**Go 1.26+** と、稼働中のローカル [HStong OpenAPI Gateway](https://quant-open.hstong.com/api-docs/)
（またはリポジトリ内の [Mock Gateway](./docs/mock-gateway.md)）が必要です。SDK は
Gateway をインストール・起動・再配布しません。既定で `http://127.0.0.1:11111`（HTTP）
と `127.0.0.1:11112`（TCP 配信）に接続します。

## クイックスタート

クライアントは一度構築して共有し、プログラム終了時にクローズします。クライアントは
並行利用に対して安全です。

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

	// 1. HSTONG_* から設定を解決します（「設定」を参照）。
	c, err := client.New(client.WithEnv())
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	// 2. 取引セッションにログインします（HSTONG_TRADE_PASSWORD が必要）。
	session := hstong.NewSessionManager(c)
	if err := session.Login(ctx); err != nil {
		log.Fatalf("trade login: %v", err)
	}
	defer session.Logout(ctx)

	// 3. 相場を 1 件取得します。
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

	// 4. 本日の real 注文を照会します（要認証）。
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

TCP チャネル経由で相場配信トピックを購読します（
`github.com/shing1211/hstongapi4go/pkg/hstong/stream` をインポート）：

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

全サーフェスの実行可能で、コンパイルに資格情報が不要なプログラムが
[`examples/`](./examples/README.md) にあります：`quickstart`、`market-data`、`trading`、
`futures`、`algo`、`streaming`。

## 機能マトリクス

エンドポイント件数は [docs/SPEC.md](./docs/SPEC.md) のみから取得しています。すべての
ルートは `POST http://127.0.0.1:11111<route>` です。

| サーフェス | パッケージ | エンドポイント数 |
|---------|---------|-----------|
| マーケットデータ取得 | `pkg/hstong/market` | 9 |
| 相場購読 / 解除 | `pkg/hstong/market` | 2 |
| 取引セッション | `pkg/hstong` | 2 |
| 取引資産 / ポジション | `pkg/hstong/trade` | 5 |
| 取引注文 | `pkg/hstong/trade` | 13 |
| 取引配信購読 | `pkg/hstong/trade` | 2 |
| アルゴ / ストラテジー | `pkg/hstong/algo` | 7 |
| 先物 | `pkg/hstong/future` | 11 |
| **HTTP 合計** | | **51** |

相場配信トピック（11）：`11`、`35`（クォート）；`14`、`27`、`28`、`37`（ティック）；
`16`（ブローカーキュー）；`17`、`25`、`26`、`36`（板情報）。[docs/SPEC.md §3](./docs/SPEC.md)
を参照してください。取引と先物の注文ステータス配信は同一 TCP チャネル上の
`TradeStockDeliverNotify` ファミリを使用します。

ハイブリッドコーデックは**エンドポイント単位**です。相場 HTTP ボディは型付き DTO を
`encoding/json` でデコードし、TCP 配信ペイロードはバイナリ protobuf で扱います。
取引・先物・アルゴ・資産・セッションのボディは手書きの JSON 構造体です。金額と数量は
`string`（または `json.Number`）で、`float64` は使いません。注文、先物、アルゴの変更は
1 回だけ試行し、自動再試行しません。

## 設定

`client.New` は文書化された既定値に対して関数オプションを順に適用します。以下の各
`HSTONG_*` 変数は `client.WithEnv()` が読み取ります。`WithEnv` の後に置いた明示的な
オプションは環境変数を上書きします。

| 変数 | 形式 | 既定値 | 用途 |
|----------|--------|---------|---------|
| `HSTONG_GATEWAY_URL` | 絶対 `http`/`https` URL | `http://127.0.0.1:11111` | Gateway HTTP ルート |
| `HSTONG_PUSH_ADDR` | `host:port` | `127.0.0.1:11112` | Gateway TCP 配信アドレス |
| `HSTONG_TIMEOUT` | Go duration（`10s`、`1500ms`） | `10s` | リクエスト単位のタイムアウトとエンベロープ `timeout_sec` |
| `HSTONG_TRADE_PASSWORD` | 平文 | 未設定 | 取引パスワード。`TradeLogin` 前に暗号化し、ログに残しません |
| `HSTONG_VERIFY_PUSH` | `strconv.ParseBool` | `false` | 任意で配信フレームの `SHA1WithRSA` 検証を有効化 |

実 Gateway に対する統合テストは既定でスキップされ、独自の変数を使います
（[`test/integration/README.md`](./test/integration/README.md) を参照）：

| 変数 | 必須 | 既定値 | 用途 |
|----------|----------|---------|---------|
| `HSTONG_INTEGRATION` | はい（ゲート） | — | `1` でないと全テストがスキップされます |
| `HSTONG_GATEWAY_URL` | いいえ | `http://127.0.0.1:11111` | Gateway HTTP ルート |
| `HSTONG_PUSH_ADDR` | いいえ | `127.0.0.1:11112` | Gateway TCP 配信アドレス |
| `HSTONG_TRADE_PASSWORD` | セッション/取引テストで必要 | — | 平文の取引パスワード |
| `HSTONG_TEST_SYMBOL` | いいえ | `00700.HK` | テスト対象の銘柄 |
| `HSTONG_TEST_EXCHANGE` | いいえ | `K` | `K` 香港、`P` 米国、`v` 深セン、`t` 上海 |
| `HSTONG_VERIFY_PUSH` | いいえ | オフ | 配信署名検証を有効化 |
| `HSTONG_PLACE_ORDERS` | いいえ | オフ | 注文変更テストを有効にするには `1` が必要 |

## パッケージ構成

```
hstongapi4go/
├── client/            # コアクライアント：オプション、環境変数、ルート、コーデック、Close
├── pkg/hstong/        # 公開マネージャ：セッション + market/trade/future/algo/stream
├── pkg/types/         # 列挙、ステータスコード、プラットフォーム公開鍵
├── pkg/domain/        # v-next: decimal value types, typed IDs, DTO mappers
├── pkg/services/      # v-next: market/account/trading use cases
├── pkg/transport/     # v-next: HTTP adapter (deadlines, caps, retry)
├── internal/          # 内部：transport、push、crypto、errs、resilience、logging、metrics
├── gen/               # 生成された protobuf コード（編集禁止）
├── proto/             # 同梱 .proto ソース + 来歴
├── test/mockgateway/  # オフライン mock HTTP + TCP 配信サーバー
├── test/e2e/          # 全エンドポイントの SDK 対モックテスト
├── test/integration/  # 環境変数でゲートされた実 Gateway テスト（既定でスキップ）
├── cmd/               # 単体バイナリ（hstong-mock-gateway）
├── examples/          # 実行可能な例（mock 対応）
├── scripts/           # ビルドと検証スクリプト
└── docs/              # MkDocs サイト、SPEC、ADR、DESIGN、LEGACY
```

## ドキュメント

- ドキュメントサイト：<https://shing1211.github.io/hstongapi4go/>
- 正式な API 索引と件数：[docs/SPEC.md](./docs/SPEC.md)
- アーキテクチャ：[ARCHITECTURE.md](./ARCHITECTURE.md) · [docs/DESIGN.md](./docs/DESIGN.md)
- 意思決定：[docs/adr/README.md](./docs/adr/README.md)（ADR 0001–0011）
- レガシープロトコル（文書化のみ、**未実装**）：[docs/LEGACY.md](./docs/LEGACY.md)
- 免責事項：[DISCLAIMER.md](./DISCLAIMER.md)

**レガシーのプラットフォーム直結プロトコル** —— `/hs/v2/login`、
`/hs/config/queryServer`、開発者 RSA 署名による直接 151 バイト socket、AES-ECB 動的
キー、ハートビート、デバイスバインディング —— は参照用に文書化されているだけで、
本 SDK では**実装されていません**。[docs/LEGACY.md](./docs/LEGACY.md) を参照してください。

## ビルドとテスト

```bash
make help            # ターゲット一覧
make build           # go build ./...
make check           # fmt + vet + money-check + 単体テスト
make test-race       # go test -race -count=1 ./...
make test-integration  # 環境変数でゲートされた実 Gateway テスト（HSTONG_* が必要）
make proto-verify    # gen/ が proto/ から乖離していれば失敗
make docs-check      # markdown リンクチェック + mkdocs build --strict
make mock-gateway    # 単体のモック Gateway を実行
```

資格情報なしで必ず通る直接コマンド：

```bash
go build ./...
go vet ./...
gofmt -l .            # 何も出力しないこと
go test ./...
go test -race -count=1 ./...
```

単体テストはオフラインで資格情報不要です。統合テストはネットワークや資格情報を必要と
する唯一のテストです。

## コントリビュート

[CONTRIBUTING.md](./CONTRIBUTING.md) を参照してください。すべてのコミットは DCO 署名
（`git commit -s`）が必要で、run と task ID を参照する Conventional Commits に従います。
新しい実行時依存には ADR が必要です。`gen/` の生成コードは編集しないでください。注文
変更を自動再試行しないでください。

## セキュリティ

[SECURITY.md](./SECURITY.md) を参照してください。SDK はプラットフォームの資格情報も
開発者の秘密鍵も保持しません。同梱のプラットフォーム公開鍵は公開参照データです。SDK が
扱う唯一の機微な値は平文の取引パスワードで、これは通信路上で暗号化し、ログに残しません。

## ライセンス

[Apache License 2.0](./LICENSE)。[NOTICE](./NOTICE)、
[DISCLAIMER.md](./DISCLAIMER.md)、[THIRD_PARTY_NOTICES.md](./THIRD_PARTY_NOTICES.md) も
参照してください。
