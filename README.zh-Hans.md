# hstongapi4go

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go" alt="Go">
  <img src="https://img.shields.io/badge/License-Apache%202.0-blue?style=flat-square" alt="许可证">
  <img src="https://img.shields.io/badge/Status-alpha-blue?style=flat-square" alt="状态">
  <img src="https://img.shields.io/badge/Gateway-v2.4.1-brightgreen?style=flat-square" alt="Gateway 版本">
  <img src="https://img.shields.io/badge/Protobuf-v2.2.0-blueviolet?style=flat-square" alt="Protobuf 包版本">
  <img src="https://img.shields.io/badge/Endpoints-51-orange?style=flat-square" alt="HTTP 端点">
  <img src="https://img.shields.io/badge/Push%20topics-11-yellowgreen?style=flat-square" alt="行情推送主题">
  <a href="https://shing1211.github.io/hstongapi4go/"><img src="https://img.shields.io/badge/Docs-GitHub%20Pages-97CAFF?style=flat-square&logo=github" alt="文档"></a>
</p>

> **非官方 & 早期预览版。** `hstongapi4go` 是面向 HStong（华盛）Quant OpenAPI
> **本地 Gateway** 的社区 Go SDK。它**与** HStong / 华盛及其任何子公司**无隶属、
> 授权、背书或赞助关系**。"HStong"、"华盛"、"华盛通"及相关名称和商标归各自所有者
> 所有，此处仅用于描述性和互操作性目的。参见 [DISCLAIMER.md](./DISCLAIMER.md)。

> **Go 原生 · 类型安全 · Gateway 优先。** 面向本地 HStong Gateway 的惯用 Go 客户端
> —— 行情数据、交易、期货、算法与实时推送 —— 金额和数量始终以字符串保存，订单变更
> 绝不自动重试。

[English](./README.md) · [简体中文](./README.zh-Hans.md) · [繁體中文](./README.zh-Hant.md) · [日本語](./README.ja.md) · [한국어](./README.ko.md) · [Español](./README.es.md)

> 本文件是英文 [README](./README.md) 的社区翻译。**英文版本为准。**
> 同步于 / Last synced: 2026-09-25

## 目录

- [状态](#状态)
- [安装](#安装)
- [快速开始](#快速开始)
- [功能矩阵](#功能矩阵)
- [配置](#配置)
- [包结构](#包结构)
- [文档](#文档)
- [构建与测试](#构建与测试)
- [贡献](#贡献)
- [安全](#安全)
- [许可证](#许可证)

---

## 状态

| 项目 | 状态 |
|------|------|
| 协议核心（HTTP 信封、路由别名、混合编解码器、类型化错误） | 已实现 |
| 行情数据拉取（9 个端点） | 已实现 |
| 行情订阅 + TCP 推送（11 个主题） | 已实现 |
| 交易会话（登录/登出、保活、单飞重新登录） | 已实现 |
| 交易资产 / 持仓（5）与订单（13） | 已实现 |
| 交易推送订阅 + 订单状态解码器 | 已实现 |
| 期货（11 个端点 + 推送） | 已实现 |
| 算法 / 策略（7 个端点） | 已实现 |
| 基于 channel 的流式 API（`Updates()` / `Errors()`） | 已实现 |
| Mock Gateway（51 条 HTTP 路由 + TCP 推送）与独立二进制 | 已实现 |
| 加固（限流、重试预算、熔断、日志、指标） | 已实现 |
| 文档（README、MkDocs 站点、ADR、SPEC、LEGACY） | 已实现 |
| 离线测试 + 全端点 SDK 对 mock 的端到端测试 | 已实现 |
| 针对真实 Gateway 的集成测试 | 已编写并按环境变量门控；待用户实际运行确认 |
| 发布（GitHub + Gitee） | v0.1.12 ✓ |

全部 51 个 HTTP 端点和 11 个行情推送主题均已实现。计数以
[docs/SPEC.md](./docs/SPEC.md) 为准；请勿在其他地方手动修改。

## 安装

```bash
go get github.com/shing1211/hstongapi4go
```

需要 **Go 1.26+** 以及正在运行的本地 [HStong OpenAPI Gateway](https://quant-open.hstong.com/api-docs/)
（或仓库内的 [Mock Gateway](./docs/mock-gateway.md)）。SDK 从不安装、启动或再分发
Gateway；默认连接 `http://127.0.0.1:11111`（HTTP）和 `127.0.0.1:11112`（TCP 推送）。

## 快速开始

构建一次客户端，共享使用，并在程序退出时关闭。客户端可安全并发使用。

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

	// 1. 从 HSTONG_* 解析配置（见「配置」章节）。
	c, err := client.New(client.WithEnv())
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	// 2. 登录交易会话（需要 HSTONG_TRADE_PASSWORD）。
	session := hstong.NewSessionManager(c)
	if err := session.Login(ctx); err != nil {
		log.Fatalf("trade login: %v", err)
	}
	defer session.Logout(ctx)

	// 3. 请求一条行情报价。
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

	// 4. 查询今日真实订单（需认证）。
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

通过 TCP 通道订阅行情推送主题（导入
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

[`examples/`](./examples/README.md) 中提供覆盖每个接口的、可运行且无需凭据即可编译的程序：
`quickstart`、`market-data`、`trading`、`futures`、`algo` 和 `streaming`。

## 功能矩阵

端点计数仅取自 [docs/SPEC.md](./docs/SPEC.md)。所有路由均为
`POST http://127.0.0.1:11111<route>`。

| 接口 | 包 | 端点数 |
|------|-----|--------|
| 行情数据拉取 | `pkg/hstong/market` | 9 |
| 行情订阅 / 取消订阅 | `pkg/hstong/market` | 2 |
| 交易会话 | `pkg/hstong` | 2 |
| 交易资产 / 持仓 | `pkg/hstong/trade` | 5 |
| 交易订单 | `pkg/hstong/trade` | 13 |
| 交易推送订阅 | `pkg/hstong/trade` | 2 |
| 算法 / 策略 | `pkg/hstong/algo` | 7 |
| 期货 | `pkg/hstong/future` | 11 |
| **HTTP 合计** | | **51** |

行情推送主题（11）：`11`、`35`（报价）；`14`、`27`、`28`、`37`（逐笔）；
`16`（经纪队列）；`17`、`25`、`26`、`36`（订单簿）。参见
[docs/SPEC.md §3](./docs/SPEC.md)。交易与期货的订单状态推送使用同一 TCP 通道上的
`TradeStockDeliverNotify` 系列。

混合编解码器是**按端点**的：行情 HTTP 响应体通过 `encoding/json` 解码为类型化 DTO，
TCP 推送载荷使用二进制 protobuf；交易、期货、算法、资产和会话响应体为手写 JSON
结构体。金额和数量为 `string`（或 `json.Number`），绝不是 `float64`。订单、期货和
算法变更只发出一次尝试，绝不自动重试。

## 配置

`client.New` 按顺序在文档化默认值之上应用函数式选项。下面每个 `HSTONG_*` 变量都由
`client.WithEnv()` 读取；放在 `WithEnv` 之后的显式选项会覆盖环境变量。

| 变量 | 格式 | 默认值 | 用途 |
|----------|--------|---------|---------|
| `HSTONG_GATEWAY_URL` | 绝对 `http`/`https` URL | `http://127.0.0.1:11111` | Gateway HTTP 根地址 |
| `HSTONG_PUSH_ADDR` | `host:port` | `127.0.0.1:11112` | Gateway TCP 推送地址 |
| `HSTONG_TIMEOUT` | Go duration（`10s`、`1500ms`） | `10s` | 单请求超时与信封 `timeout_sec` |
| `HSTONG_TRADE_PASSWORD` | 明文 | 未设置 | 交易密码；在 `TradeLogin` 前加密，绝不记录日志 |
| `HSTONG_VERIFY_PUSH` | `strconv.ParseBool` | `false` | 可选开启推送帧 `SHA1WithRSA` 验签 |

针对真实 Gateway 的集成测试默认跳过，并有自己的变量（参见
[`test/integration/README.md`](./test/integration/README.md)）：

| 变量 | 是否必需 | 默认值 | 用途 |
|----------|----------|---------|---------|
| `HSTONG_INTEGRATION` | 是（门控） | — | 必须为 `1`，否则所有测试跳过 |
| `HSTONG_GATEWAY_URL` | 否 | `http://127.0.0.1:11111` | Gateway HTTP 根地址 |
| `HSTONG_PUSH_ADDR` | 否 | `127.0.0.1:11112` | Gateway TCP 推送地址 |
| `HSTONG_TRADE_PASSWORD` | 会话/交易测试需要 | — | 明文交易密码 |
| `HSTONG_TEST_SYMBOL` | 否 | `00700.HK` | 被测标的 |
| `HSTONG_TEST_EXCHANGE` | 否 | `K` | `K` 港股、`P` 美股、`v` 深市、`t` 沪市 |
| `HSTONG_VERIFY_PUSH` | 否 | 关闭 | 启用推送签名验证 |
| `HSTONG_PLACE_ORDERS` | 否 | 关闭 | 必须为 `1` 才启用订单变更测试 |

## 包结构

```
hstongapi4go/
├── client/            # 核心客户端：选项、环境变量、路由、编解码器、Close
├── pkg/hstong/        # 公开管理器：会话 + market/trade/future/algo/stream
├── pkg/types/         # 枚举、状态码、平台公钥
├── pkg/domain/        # v-next: decimal value types, typed IDs, DTO mappers
├── pkg/services/      # v-next: market/account/trading use cases
├── pkg/transport/     # v-next: HTTP adapter (deadlines, caps, retry)
├── internal/          # 私有：transport、push、crypto、errs、resilience、logging、metrics
├── gen/               # 生成的 protobuf 代码（请勿编辑）
├── proto/             # 内置 .proto 源 + 来源说明
├── test/mockgateway/  # 离线 mock HTTP + TCP 推送服务器
├── test/e2e/          # 全端点 SDK 对 mock 测试
├── test/integration/  # 按环境变量门控的真实 Gateway 测试（默认跳过）
├── cmd/               # 独立二进制（hstong-mock-gateway）
├── examples/          # 可运行示例（适配 mock）
├── scripts/           # 构建与校验脚本
└── docs/              # MkDocs 站点、SPEC、ADR、DESIGN、LEGACY
```

## 文档

- 文档站点：<https://shing1211.github.io/hstongapi4go/>
- 规范 API 索引与计数：[docs/SPEC.md](./docs/SPEC.md)
- 架构：[ARCHITECTURE.md](./ARCHITECTURE.md) · [docs/DESIGN.md](./docs/DESIGN.md)
- 决策：[docs/adr/README.md](./docs/adr/README.md)（ADR 0001–0011）
- 旧版协议（已文档化，**未实现**）：[docs/LEGACY.md](./docs/LEGACY.md)
- 免责声明：[DISCLAIMER.md](./DISCLAIMER.md)

**旧版直连平台协议** —— `/hs/v2/login`、`/hs/config/queryServer`、使用开发者 RSA
签名的直连 151 字节 socket、AES-ECB 动态密钥、心跳与设备绑定 —— 仅为参考而文档化，
本 SDK **未实现**。参见 [docs/LEGACY.md](./docs/LEGACY.md)。

## 构建与测试

```bash
make help            # 列出目标
make build           # go build ./...
make check           # fmt + vet + money-check + 单元测试
make test-race       # go test -race -count=1 ./...
make test-integration  # 按环境变量门控的真实 Gateway 测试（需要 HSTONG_*）
make proto-verify    # 若 gen/ 与 proto/ 漂移则失败
make docs-check      # markdown 链接检查 + mkdocs build --strict
make mock-gateway    # 运行独立的 mock Gateway
```

无需凭据即可通过的直接命令：

```bash
go build ./...
go vet ./...
gofmt -l .            # 必须无输出
go test ./...
go test -race -count=1 ./...
```

单元测试离线且无需凭据。集成测试是唯一可能需要网络或凭据的测试。

## 贡献

参见 [CONTRIBUTING.md](./CONTRIBUTING.md)。所有提交必须 DCO 签署
（`git commit -s`）并遵循引用 run 和 task ID 的 Conventional Commits。新的运行时依赖
需要 ADR；绝不编辑 `gen/` 下的生成代码；绝不对订单变更自动重试。

## 安全

参见 [SECURITY.md](./SECURITY.md)。SDK 不持有平台凭据，也不持有开发者私钥；内置的
平台公钥为公开参考数据。SDK 处理的唯一敏感值是明文交易密码，它在传输中加密且绝不
记录日志。

## 许可证

[Apache License 2.0](./LICENSE)。参见 [NOTICE](./NOTICE)、
[DISCLAIMER.md](./DISCLAIMER.md) 和 [THIRD_PARTY_NOTICES.md](./THIRD_PARTY_NOTICES.md)。
