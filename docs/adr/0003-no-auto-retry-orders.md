# 0003 — Never auto-retry order mutations

- Status: Accepted
- Date: 2026-09-21

## Context

Retries are desirable for transient failures (network resets, `1011` service busy,
`1015` interface call timeout, `1018` reconnecting). But HStong order, futures, and
algo endpoints are **not idempotent**: resubmitting after a timeout can create a
duplicate order — a serious financial error. HStong provides no idempotency key to
make submission safe to retry. The Gateway's own `timeout_sec` request field means a
call can time out after the request reached the platform but before the response was
read, leaving the true state unknown (`1007` duplicate submission exists precisely
because of this ambiguity).

Batch cancel is also documented as capable of timing out when the day's order volume
exceeds a threshold (plan §F4), so it carries the same ambiguity.

## Decision

- The resilience layer may retry **read-only query endpoints only** (market data;
  assets/positions; fund-journey and order/entrust/deliver/condition-order queries;
  futures product/fund/holds/max-buy-sell queries; algo order/entrust queries).
- **Order mutations are never auto-retried, at any layer, under any configuration.**
  The named mutation endpoints are:
  - **Trade:** `TradeEntrust`, `TradeCancelEntrust`, `TradeBatchCancelEntrust`,
    `TradeChangeEntrust`
  - **Futures:** `FuturesEntrust`, `FuturesCancelEntrust`, `FuturesModifyEntrust`
  - **Algo:** `AlgoAddOrder`, `AlgoCancelOrder`, `AlgoCancelEntrust`,
    `AlgoChangeOrder`, `AlgoActionOrder`
- On an ambiguous failure (timeout after the request was sent), the SDK returns a
  typed error instructing the caller to **reconcile** before resubmitting, by
  querying the real/history entrust and deliver lists for the affected account.
- The mutation list is a closed set in code, not a caller-supplied flag. A caller
  cannot enable global retry for mutations.

## Consequences

- Transient failures on an order mutation surface as errors instead of silently
  retrying, so the caller always retains control.
- Callers must implement reconciliation for ambiguous submission outcomes; the
  examples and docs must show this.
- Tests assert exactly **one outbound request** per mutation (T20) and the resilience
  layer excludes mutations (T30).
- Query retries and re-login (which is not an order mutation) remain automatic.

## Alternatives considered

| Alternative | Why rejected |
|-------------|--------------|
| Retry idempotent-looking mutations (e.g. cancel) | Cancel/change affect live state and are not guaranteed idempotent; a retried cancel can hit a reused identifier or report the wrong terminal state. |
| Client-supplied idempotency keys | HStong provides no idempotency-key field in the wire protocol. |
| Retry with a short, fixed budget | Ambiguity is not time-bounded; a second attempt during the ambiguity window is exactly the dangerous case. |

## References

- Plan: [plan.md](../runs/2026-09-21-hstong-full-surface/plan.md) §Assumption 4, §F4, §R5
- Related: [0001](./0001-gateway-transport.md), [0002](./0002-hybrid-codec.md)
