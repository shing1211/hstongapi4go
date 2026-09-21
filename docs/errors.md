# Error Codes

When the Gateway answers `{"ok":false,"err":"..."}`, the SDK classifies the
failure into a typed error that carries the operation label, the status code, and
a category, and preserves `errors.Is` / `errors.As` traversal through the cause.

## Gateway status codes

| Code | Meaning | Category | Retryable (queries only) |
|------|---------|----------|:------------------------:|
| `0000` | success | success | — |
| `1001` | unknown system error | api | no |
| `1002` | signature error | api | no |
| `1003` | data encryption error | api | no |
| `1004` | socket not initialized | connection | no |
| `1005` | endpoint deprecated | api | no |
| `1006` | user not authorized | account | no |
| `1007` | duplicate submission | trading | no |
| `1008` | call failed | api | no |
| `1009` | endpoint not found | api | no |
| `1010` | illegal request | api | no |
| `1011` | service busy, retry later | rate_limit | yes |
| `1012` | not logged in | account | no (re-login) |
| `1013` | session displaced | account | no (re-login) |
| `1014` | login timeout | account | no (re-login) |
| `1015` | call timeout | timeout | yes |
| `1016` | invalid parameter | api | no |
| `1017` | long-connection establishment failed | connection | yes |
| `1018` | reconnecting, retry later | connection | yes |
| `20033` | futures trade login timeout | account | no (re-login) |
| `40001` | query product info failed | api | no |
| `40002` | query contract info failed | api | no |

"The Gateway may report a bare code (`\"1012\"`), a code followed by text
(`\"1012 not logged in\"`), or free text with no code. The SDK recovers a
documented code when one is present and otherwise returns the trimmed text.

## Retryability

Only **read-only query endpoints** may be retried by a caller or a resilience
layer. `1011`, `1015`, `1017`, `1018`, and transport connection/timeout failures
are the retryable conditions.

**Order mutations are never retried, at any layer, under any configuration**
(ADR 0003). This includes:

- Trade `Entrust`, `CancelEntrust`, `BatchCancelEntrust`, `ChangeEntrust`
- Futures `Entrust`, `CancelEntrust`, `ModifyEntrust`
- Algo `AddOrder`, `CancelOrder`, `CancelEntrust`, `ChangeOrder`, `ActionOrder`

On an ambiguous failure (for example a timeout after the request was sent),
reconcile before resubmitting by querying the real/history entrust and deliver
lists for the affected account.

## Re-login conditions

`1012`, `1013`, `1014`, and `20033` indicate the session is no longer valid. The
trade manager performs a best-effort single-flight re-login and returns the
original error to the caller. See [Authentication](authentication.md).

## Public sentinel errors

These exported sentinels can be matched with `errors.Is`:

| Sentinel | Returned by | Meaning |
|----------|-------------|---------|
| `client.ErrClientClosed` | `Client.Do` after `Close` | the client was closed |
| `client.ErrUnknownRoute` | `Route.Validate` (via `Client.Do`) | route is not one of the 51 registered endpoints |
| `stream.ErrClosed` | `Connect`, `Subscribe` after `Close` | the stream client was closed |
| `stream.ErrNoClient` | `Subscribe`, `SubscribeTrade` | built without a `*client.Client` |
| `stream.ErrReconnected` | `Subscription.Errors()` | push connection re-established |
| `algo.ErrInvalidParams` | any algo method | local validation failed before any request |
| `push.ErrClosed` | internal push client | internal only |

```go
if err := c.Do(ctx, op, route, params, c.JSON(), &out); err != nil {
    if errors.Is(err, client.ErrUnknownRoute) {
        // programming error: fix the route
    }
    if errors.Is(err, client.ErrClientClosed) {
        // the client was closed
    }
}
```

## Validation errors

Managers validate locally and **send no request** when the request is malformed.
These failures use the documented `1016` invalid-parameter status so callers can
match them like any Gateway rejection.

| Validation | Rejected by |
|------------|-------------|
| empty security list / nil security | market `BasicQot` |
| `limit` outside `1..100` | market `Ticker` |
| unknown `topicId` | market `Subscribe` / `Unsubscribe` |
| missing required order fields, non-positive amount, `validDays` outside `1..100` | trade order methods |
| missing contract code, whitespace in a code, bad date, bad quantity/price | futures methods |
| missing `orderId`, out-of-set `action`, bad date, missing `sensitivity` | algo methods |

## Notes on the internal error type

The typed error lives in `internal/errs`, which is not importable outside the
module. Applications outside the module match behavior with the public sentinels
above and by inspecting the error text. Code inside the module can use
`errs.CodeOf`, `errs.CategoryOf`, `errs.Retryable`, and
`errs.ReLoginRequired`. The SDK never matches on error strings internally.
