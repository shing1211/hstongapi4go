# Design note — should `ErrReconnected` wrap the reconnect cause?

- **Run:** `2026-09-26-vnext-parity-wire`
- **Task:** A4 (`architect`) — record a decision; **no code changed**
- **Date:** 2026-09-26
- **Status:** Decided. Implementation is a follow-on task (see §6 — it is
  blocked on a second test file that this task's scope does not cover).
- **Related:** [ADR 0011](../../adr/0011-v01x-compatibility.md), task A1 in
  [todos.md](./todos.md), [ADR 0004](../../adr/0004-minimal-dependencies.md)

## 1. The defect, stated precisely

`pkg/hstong/stream/stream.go`:

| Location | What it says / does |
|----------|---------------------|
| `stream.go:30-33` | The `ErrReconnected` GoDoc: *"The error that ended the previous connection is wrapped and available with `errors.Unwrap`."* |
| `stream.go:412` | `fmt.Errorf("%w: %v", ErrReconnected, cause)` — the single `%w` binds to `ErrReconnected`; the cause is rendered by `%v`, so it becomes text. |
| `stream.go:397` | `onReconnect(cause error)` receives the cause from `push.WithOnReconnect` (`stream.go:167`). |

Measured behaviour of the notice a caller receives today:

| Query | Result |
|-------|--------|
| `errors.Is(notice, ErrReconnected)` | `true` — matches the sentinel |
| `errors.Unwrap(notice)` | `ErrReconnected` — **not** the cause |
| `errors.Is(notice, cause)` | `false` — always, for every cause |
| `errors.Is(notice, io.EOF)` and friends | `false` |
| `notice.Error()` | `"stream: push connection re-established: <cause>"` — readable, but text |

So the documented sentence is false, and the reason a caller cannot branch on
*why* the connection dropped is not that the cause is missing — it is present
and readable — but that it is **unmatchable**.

`harness_internal_test.go:299-321` already pins the implemented behaviour and
names the discrepancy in its own comment, deferring the decision to this task.
That test is the reason the defect is still here: A1 recorded it rather than
fixing it, and A4 is the decision it deferred to.

### 1.1 What the cause actually is — and one correction to the brief

The brief's motivating example is "treat a signature-verification failure
differently from a plain network reset". **A signature-verification failure can
never be the reconnect cause.** In `internal/push/client.go:445-450`, a frame
that fails `verifyFrame` is reported on the push error channel and the read
loop *continues*; the loop returns only from `SetReadDeadline` failure
(`client.go:432-435`) or a `ReadFrame` error (`client.go:437-440`). Only that
return value reaches `fireOnReconnect` (`client.go:412`). Verification failures
reach the caller by a different path entirely — `stream.go:249-271`,
`forwardPushErrors` → `broadcastError`, which puts them on the same
`Errors()` channel but as their own error, already matchable today.

This correction matters twice over:

1. It changes **what the fix buys the caller.** The cause is one of
   `io.EOF` / `io.ErrUnexpectedEOF` (short read, `frame.go:173`, `frame.go:193`),
   a `*net.OpError` (deadline expiry or a hard reset), or a framing sentinel
   from `internal/push` (`ErrShortHeader`, `ErrBadMagic`, `ErrInvalidBodyLen`,
   `ErrBodyTooLarge`, `ErrUnsupportedCompression` — `frame.go:46-55`,
   `frame.go:144-188`). The first group is **stdlib and externally matchable**:
   `errors.Is(notice, io.EOF)`, `errors.Is(notice, os.ErrDeadlineExceeded)`, and
   `errors.As(notice, &netErr); netErr.Timeout()` are all real decisions with
   real operational answers — "the Gateway hung up, expect a gap" versus "the
   read deadline expired, the feed is stale".
2. It removes the only serious objection to wrapping. A common reason to keep a
   cause as text is that `internal/` sentinels are not importable by external
   callers, so wrapping them would promise a matchability the caller cannot
   use. That objection does not apply here, because the useful part of the cause
   is stdlib.

The framing sentinels stay unmatchable from outside either way. That is
acceptable: their *messages* are the diagnostic, they are rare, and exposing
`internal/push` sentinels through a public error is not worth new public
surface.

## 2. Decision

**The code is wrong. `ErrReconnected` must wrap the reconnect cause.**

Three facts decide it:

1. **The doc is the specification and the code is the deviation.** The GoDoc
   sentence is specific, deliberate, and names the exact mechanism
   (`errors.Unwrap`). A `%v` in a format string is not a position. When a
   doc comment and a format verb disagree, the doc wins unless the code shows
   intent the doc denies — and here it shows none.
2. **The fix is strictly capability-adding.** Today the cause is unmatchable.
   Nothing a caller can express today stops being expressible after the change
   (§3).
3. **The capability is worth having.** Per §1.1 the cause carries a
   `stdlib`-matchable distinction between a routine hangup and a stale feed, and
   the one thing a caller may *not* do with a text-only cause — parse the
   message — is forbidden by AGENTS.md. Text-only is not a weaker promise, it
   is an unusable one.

Correcting the doc instead (option D, §4) would mean rewriting a deliberate
promise, weakening the API, and — under AGENTS.md's own rule against matching
on error strings — leaving callers with no supported way to branch on the cause
at all.

### 2.1 Mechanism: an unexported notice type, not a second `%w`

The recommendation is a small unexported error type in
`pkg/hstong/stream/stream.go`:

```go
// reconnectNotice reports a completed push reconnect. It matches
// ErrReconnected under errors.Is and unwraps to the error that ended the
// previous connection, so a caller can branch on the cause with errors.Is or
// errors.As without parsing the message.
type reconnectNotice struct{ cause error }

// newReconnectNotice builds the notice pushed on a subscription's Errors
// channel after a reconnect. A nil cause yields the bare sentinel.
func newReconnectNotice(cause error) error {
    if cause == nil {
        return ErrReconnected
    }
    return &reconnectNotice{cause: cause}
}

func (e *reconnectNotice) Error() string { return ErrReconnected.Error() + ": " + e.cause.Error() }
func (e *reconnectNotice) Is(target error) bool { return target == ErrReconnected }
func (e *reconnectNotice) Unwrap() error { return e.cause }
```

and `stream.go:412` becomes `sub.sendErr(newReconnectNotice(cause))`.

Why this and not `fmt.Errorf("%w: %w", ErrReconnected, cause)`: the two differ in
exactly one observable way, and it is the observable the doc promises. Measured,
with `cause = io.ErrUnexpectedEOF`, all three candidates produce a
**byte-identical** message, so message stability is not a discriminator:

| | `errors.Is(·, ErrReconnected)` | `errors.Is(·, cause)` | `errors.Unwrap(·)` |
|---|---|---|---|
| today (`%w: %v`) | `true` | `false` | `ErrReconnected` |
| `%w: %w` (Go 1.20+ multi-wrap) | `true` | `true` | **`nil`** |
| `*reconnectNotice` | `true` | `true` | **`cause`** |

`fmt.Errorf` with two `%w` verbs returns a value implementing
`Unwrap() []error`, not `Unwrap() error`. `errors.Unwrap` only understands the
singular form, so it returns `nil`. Option A therefore makes the documented
sentence *more* wrong, and leaves a silent-wrong-branch trap: a caller who
follows the current doc — `u := errors.Unwrap(notice); if errors.Is(u, io.EOF)`
— gets a `nil` `u` and falls into the "unknown cause" branch forever, with no
error to notice. The notice type satisfies the doc as written, which is the
whole point of the exercise.

The `Is` method is what stands in for the `%w` on the sentinel. It is one line
and it is the standard Go idiom for "this error *is* sentinel X and also wraps
cause Y" (the same shape as `fs.PathError` and `net.OpError`).

**`newReconnectNotice(nil)` is not defensive noise.** `fmt.Errorf("%w: %v", ...)`
prints `<nil>` for a nil cause and does not panic; `e.cause.Error()` does. A
panic in a public API on a value the SDK does not control — `push.Run` passes
`fireOnReconnect`'s argument straight through, and `push.Client.report`
(`client.go:529-533`) explicitly guards `err == nil`, so the push package does
not treat nil as impossible — is not an acceptable trade for a two-line guard.
The nil branch is reachable from a test without a live Gateway, so the 100%
coverage A1 established is preserved.

No new dependency: `errors`, `fmt` semantics only (AGENTS.md rule 8). No new
exported identifier, so nothing is added to the API surface ADR 0011 protects.

## 3. ADR 0011 analysis

[ADR 0011](../../adr/0011-v01x-compatibility.md) covers `pkg/hstong/stream`
explicitly (its surface table, line 32). Each guarantee, applied to the
recommended change:

| Guarantee | Verdict |
|-----------|---------|
| 1. No breaking type changes (no field removals, no type changes, no function signature changes) | **Held.** `ErrReconnected` stays `var … = errors.New(…)`, same value, same comparability. `onReconnect` keeps its `func(error)` signature. No exported type is touched. The new type is unexported and appears in no signature. |
| 2. No breaking wire changes | **Held.** Nothing on the wire changes; this is an in-process error value. |
| 3. No new mandatory dependencies | **Held.** stdlib only. |
| 4. Mock Gateway compatibility | **Held.** `stream_test.go:308` (the end-to-end reconnect test over a fake Gateway) asserts only `errors.Is(err, stream.ErrReconnected)`, which stays `true`. No e2e test needs updating. |
| 5. `gen/` never touched | **Held.** |

**The permitted-changes table.** The behavioural change is what needs a home.
The relevant row is *"Bug fixes that change runtime behaviour — Expected; bugs
are not specified behaviour"*. That is the governing row, and it is the right
one: the doc promised wrapability, the code never delivered it, so wrapability
was never specified *behaviour* — there was no specified behaviour to break.
Adding an unexported type is separately covered by *"Adding new fields to
unexported structs"*. The change needs no waiver, no ADR amendment, and no
exception recorded anywhere.

**Direction of risk, walked in both directions.** The brief's framing is right
that risk runs one way, and it is worth being exact about why, because
"additive" is a claim about *capability* while ADR 0011 is a guarantee about
*behaviour*.

- *Capability can only grow.* `errors.Is(n, ErrReconnected)` stays `true`
  (via the `Is` method). `errors.Is(n, X)` for any cause `X` goes `false` →
  `true`. `errors.As` into any stdlib net/io type goes `false` → `true`. There
  is no query that answered `true` before and answers `false` after, except one:
  `errors.Unwrap(n) == ErrReconnected`, which today returns the sentinel and
  after the change returns the cause.
- *Could that regress a caller?* Only a caller who **pinned the bug**. Reading
  the GoDoc, a caller would expect `errors.Unwrap` to hand back the cause; a
  caller who *tested* it gets the sentinel and might have encoded
  `if errors.Unwrap(err) == stream.ErrReconnected` as a reconnect check. That
  code is broken today — it does not do what the doc says, and it breaks
  anyway on the resubscribe failures the same channel carries
  (`stream.go:422`, `stream.go:432`). It is the one behaviour the change
  moves, and it moves it from wrong to right. ADR 0011's permitted-changes
  table covers exactly this case.
- *String matching.* No message changes — verified byte-identical in §2.1. A
  caller doing `strings.Contains(err.Error(), "re-established")` or matching on
  the cause's text is unaffected. (AGENTS.md forbids string matching in library
  code, so the repository's own consumers are unaffected by construction; the
  message is preserved anyway for the out-of-repo ones and for log continuity.)
- *Repository callers.* Exactly one: `stream_test.go:308`, `errors.Is` only,
  preserved. And one in-repo assertion that pins the bug rather than the
  contract, `harness_internal_test.go:315` — see §6.

**Conclusion: not a breaking change under ADR 0011.** No waiver, no amendment.

## 4. Rejected alternatives

| Alternative | Why rejected |
|-------------|--------------|
| **Correct the doc** to stop over-promising (fix `stream.go:30-33` instead) | Rejected in §2. It deletes a deliberate promise, weakens the public API, and leaves callers with no supported way to branch on the cause — string parsing is forbidden by AGENTS.md. It also contradicts four other repository statements that all describe a wrapping notice (`docs/streaming.md:142`, `docs/observability.md:86`, `docs/errors.md:71`, `docs/streaming.md:7`). Those four stay true under either option, so they are not evidence either way — but the GoDoc is the only place the *cause* is promised, and it promises it deliberately. |
| **`fmt.Errorf("%w: %w", ErrReconnected, cause)`** (Go 1.20+ multi-wrap) | **The real contender**, and one token cheaper. Rejected because it makes `errors.Unwrap` return `nil`, falsifying the documented sentence it was meant to satisfy, and creating a silent-wrong-branch trap for exactly the callers the doc addresses. If a maintainer prefers it, it is a defensible choice — the doc sentence then has to be reworded from `errors.Unwrap` to `errors.Is`/`errors.As`, and `harness_internal_test.go` must be updated either way. It is recorded here so the follow-on does not have to re-derive it. |
| **`errors.Join(ErrReconnected, cause)`** | Rejected. `errors.Join` renders as two newline-separated lines, so the message changes shape — the one thing §2.1 established all other candidates preserve. It also gives `Unwrap() []error` (same `errors.Unwrap` → `nil` problem as multi-`%w`) and loses the "notice" prefix, so `strings.HasPrefix(msg, "stream:")` breaks. |
| **Exported `ReconnectError` with a `Cause() error` accessor** | Rejected. `errors.Unwrap` already returns the cause, so the accessor adds no capability, and it adds a permanent public type to a surface that is frozen until v1.0.0 (ADR 0011 §Deprecation). Contradicts ADR 0004's minimalism for no gain. If a caller later needs `errors.As(notice, &ReconnectError{})`, that is a v1.0.0 addition. |
| **Wrap the cause only, dropping the `ErrReconnected` sentinel** | Rejected. `errors.Is(err, stream.ErrReconnected)` is the documented, tutorial-level way to recognise a reconnect (`docs/CONTRIBUTING.md:210-227`, `docs/runbook.md:41`, `docs/observability.md:129`). Removing the sentinel match would break every caller and three doc examples. |
| **Do nothing** | Rejected. The defect is recorded in a test comment as a known-wrong state, which is a standing invitation to treat the doc as unreliable. |

## 5. Test strategy

The guarantee to protect is a *relationship between a doc sentence and a
returned value*, so the test has to assert the relationship, not the shape.
Four properties, all reachable offline with no Gateway and no timing — A1's
harness (`newTestClient`, `newHookServer`, `nextError`) already provides
`s.onReconnect(cause)` and the queued error, so nothing here needs a sleep or a
real socket.

| # | Property | Assertion | Kills |
|---|----------|-----------|-------|
| T1 | The sentinel still matches | `errors.Is(notice, ErrReconnected)` | removal of the `Is` method; reverting to `%w: %v` |
| T2 | The cause is matchable | `errors.Is(notice, cause)` | `%v` → `%w` reversal; `Unwrap` returning the sentinel |
| T3 | The documented mechanism works | `errors.Unwrap(notice) == cause` | any drift back to multi-`%w`; this is the one that distinguishes the recommendation from §4's multi-`%w` row, so it must exist |
| T4 | The cause is not text-only | `errors.As(notice, &netErr)` against a `*net.OpError` cause, or `errors.Is(notice, os.ErrDeadlineExceeded)` | a `Error()`-only implementation that keeps the text and drops the chain |
| T5 | Message continuity | `notice.Error()` equals `"stream: push connection re-established: <cause>"` — byte-identical to today | any message-shape regression from `errors.Join` or a reworded prefix |
| T6 | A nil cause does not panic | `newReconnectNotice(nil)` returns the bare `ErrReconnected` and `Error()` does not panic | removal of the nil guard, which would turn a `%v` tolerance into a panic |
| T7 | Stdlib reachability | end-to-end: the existing fake-Gateway reconnect in `stream_test.go:300-313` additionally asserts `errors.Is(err, io.EOF)` (the real cause when the peer closes) | a fix applied only to a synthetic path, or one that loses the chain under the real `net` stack |

**Why T3 and T5 are the load-bearing ones.** T2 alone is satisfied by option A
and by the recommendation, so it cannot tell the two apart — a test suite
without T3 would let a future maintainer "simplify" the notice into a
multi-`%w` and pass, silently breaking the documented mechanism. T5 is what
keeps the out-of-repo string matchers and the logs working, and it is the only
assertion that fails if someone reaches for `errors.Join`.

**Mutation check.** T1–T4 must each kill the obvious mutation:
`Is` deleted; `Unwrap` returning `ErrReconnected`; `%v` restored; `Error()`
returning a constant with no cause; the nil guard removed. T5 must kill a
`"\n"`-joined `errors.Join` message. A1 ran 24 scripted mutations at 100%; this
change must not lower that bar, and T6 exists so the nil guard is not counted as
an uncovered branch under the package's 100% gate.

**What must not be tested.** No test should assert on `err.Error()` alone to
*prove* matchability — that is the bug being fixed, and asserting it would
re-pin the defect. Matchability is asserted only through `errors.Is` /
`errors.As` / `errors.Unwrap`.

## 6. Follow-on scope (not done here)

No `.go` file was changed. Two reasons, and the first is the blocker.

**The blocker: every correct fix breaks a file this task may not touch.**
`harness_internal_test.go:310-317` — written by A1 — asserts
`errors.Unwrap(err) == ErrReconnected`, i.e. it pins the defect. Under §2.1's
table, *both* recommended mechanisms and *every* rejected-but-correct
alternative make that assertion fail (nil, or the cause). So the fix cannot be
completed inside this task's stated scope, which confines the change to
`stream.go` plus a *new* test file. Splitting it would leave the package
red on arrival, which is worse than leaving it green with the defect recorded.
The follow-on must therefore edit `harness_internal_test.go:299-321` as part of
the same change: delete the "pins the behaviour as implemented … reported as a
finding rather than fixed here" comment, and invert the `errors.Unwrap`
assertion to expect the cause. Its three subtests
(`reconnect_internal_test.go:141`, `:178`, `:189`, `:214`, `:225`) then inherit
T1–T3 and T5 for free, through `assertReconnectNotice`.

The follow-on, in order:

1. Delete or invert the `errors.Unwrap` assertion and the discrepancy comment
   in `harness_internal_test.go:299-321`; have it delegate to the new
   assertions in §5.
2. Add `reconnectNotice` + `newReconnectNotice` to
   `pkg/hstong/stream/stream.go`; replace `stream.go:412` with
   `sub.sendErr(newReconnectNotice(cause))`.
3. Add a new `package stream` test file (e.g.
   `reconnect_cause_internal_test.go`) covering T1–T6, including the nil-cause
   branch and the byte-identical message.
4. Extend the end-to-end fake-Gateway reconnect in `stream_test.go:306-313`
   with T7.
5. Record the new file's mutation results alongside A1's 24 in `todos.md`.
6. Re-run: `gofmt -l .`, `go vet ./pkg/hstong/stream/`,
   `go test -count=1 ./pkg/hstong/stream/`, `go test -race -count=1 ./...`,
   and the package coverage gate — which must still be ≥90% and, per A1,
   100.0%.

Steps 2–4 are mechanical and fully specified by §2.1 and §5. Step 1 is a
four-line edit to another task's test and needs a maintainer to accept that
A1's pinned assertion was a placeholder, not a contract.

## 7. Verification run for this task

No source file changed, so these are a no-change baseline, not a proof of a fix.

Run with `$env:GOTMPDIR="C:\Users\Tchan\AppData\Local\Temp\opencode"`.
`make` is not installed, so the targets are invoked directly; `docs-check` is
`check_links.py` + `check_i18n.py` + `mkdocs build --strict` (Makefile:113-116),
so all three are run.

```
$ gofmt -l .
(no output)

$ go vet ./pkg/hstong/stream/
exit=0

$ go test -count=1 ./pkg/hstong/stream/
ok  	github.com/shing1211/hstongapi4go/pkg/hstong/stream	0.604s
exit=0

$ python scripts/check_links.py
check_links: scanned 100 Markdown files, 0 unresolved link(s)
exit=0

$ python scripts/check_i18n.py
i18n OK: 6 languages consistent
exit=0

$ mkdocs build --strict
INFO    -  Documentation built in 1.45 seconds
exit=0
```

`check_links.py` reports 100 files, up from 99 before this note was added, so
this file's four relative links (`../../adr/0011-v01x-compatibility.md`,
`../../adr/0004-minimal-dependencies.md`, `./todos.md`) were resolved on disk.

One caveat on the `mkdocs` line: the 16 `INFO - Doc file … contains a link to
'runs/…' which is excluded from the built site` lines are pre-existing and
`INFO`, not warnings, so `--strict` is satisfied. They include
`adr/0008-decimal-financial-types.md` linking to this run folder's
`design-tick-model.md`, which establishes that **`docs/runs/**` is excluded from
the mkdocs nav**. `mkdocs build --strict` therefore did not build this file;
`check_links.py` is what validated its links. That is the same situation as
`design-tick-model.md`, and `check_i18n.py` is unaffected because the run folder
has no translations.

```
$ git status --short
?? docs/runs/2026-09-26-vnext-parity-wire/design-reconnect-cause.md
?? pkg/hstong/algo/query_failure_test.go
?? pkg/hstong/algo/validation_matrix_test.go
```

Only this note is mine. The two `pkg/hstong/algo` files are another agent's
untracked work, present before this task started and untouched by it. No
`.go` file, and nothing in `pkg/hstong/algo`, `pkg/services`,
`pkg/hstong/trade`, `internal/`, `gen/`, `scripts/`, or any workflow, was
modified. Nothing was committed or pushed.
