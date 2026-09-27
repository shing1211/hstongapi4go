# Design note — the SPEC ↔ v-next service parity guard

- **Run:** `2026-09-26-vnext-parity-wire`
- **Task:** C1 (`architect`) — record a decision; **no Go file changed**
- **Date:** 2026-09-26
- **Status:** Decided. Implementation is C2 (report mode); enforcement is C14.
- **Related:** [ADR 0010](../../adr/0010-vnext-layered-architecture.md),
  [ADR 0011](../../adr/0011-v01x-compatibility.md),
  [ADR 0004](../../adr/0004-minimal-dependencies.md), hard rules 5 and 8 in
  [AGENTS.md](../../../AGENTS.md), tasks C2 and C14 in [todos.md](./todos.md)

## 1. The gap, measured

`docs/SPEC.md:122` declares `**Total: 51 HTTP endpoints** (9 + 2 + 2 + 5 + 13 + 2 +
7 + 11)`. `client/routes.go:28-95` declares exactly 51 `client.Route` constants in
the same order. `pkg/services` names **29** of them, so **22 are unwired**:

| SPEC group (§2.x) | Declared | Referenced by `pkg/services` | Unwired | Owning task |
|-------------------|---------:|-----------------------------:|--------:|-------------|
| 2.1 Market pull | 9 | 9 | 0 | — (shipped) |
| 2.2 Market subscription | 2 | 2 | 0 | — (shipped) |
| 2.3 Trade session | 2 | 0 | **2** | C13 |
| 2.4 Trade assets / positions | 5 | 5 | 0 | — (shipped) |
| 2.5 Trade orders | 13 | 13 | 0 | — (shipped) |
| 2.6 Trade push subscribe | 2 | 0 | **2** | **none** — see §1.1 |
| 2.7 Algo / strategy | 7 | 0 | **7** | C7–C9 |
| 2.8 Futures | 11 | 0 | **11** | C3–C6 |
| **Total** | **51** | **29** | **22** | |

Per file: `pkg/services/market.go` 11, `trading.go` 13, `account.go` 5. The same 29
names appear in `pkg/services/*_test.go` and nowhere else outside them, so today a
"scan the package" guard and a "scan the package, tests excluded" guard would report
the same 29. That is a coincidence, not a property — see §4.1.

### 1.1 The backlog is short by two endpoints

C3–C6 (11 futures) and C7–C9 (7 algo) account for 18. C13 composes `internal/auth`,
which is where login/logout belong. That is 20 of 22.

**`RouteTradeSubscribe` and `RouteTradeUnsubscribe` are owned by no task in the run.**
C10–C12 are `PushOrchestration`, which [ADR 0010](../../adr/0010-vnext-layered-architecture.md)
places as a *service* and which C11 scopes to `internal/push.Client` — a TCP client
speaking protobuf. It will never reference a `client.Route`. The HTTP half of trade
push (registering the session for order-status notification) is a separate call, and
nothing in the run names it. `PushOrchestration` is the natural owner — it is the one
service whose job is push — but that has to be **decided in C10's design note**, or
C14 turns red on two endpoints nobody was asked to implement. The guard is what makes
that visible on day one instead of at C14.

I checked the obvious suspicion — that these two are reachable only by push
subscription and so have no `client.Do` call to find — and it is **false**.
`docs/SPEC.md:91-92` lists them as ordinary `POST` rows (32 and 33), and
`pkg/hstong/trade/push.go:33,41` issues them through `Manager.call` with an empty
`struct{}{}` param. `RouteHqSubscribe` / `RouteHqUnsubscribe` are the same shape and
are already wired at `pkg/services/market.go:429,446`. So the market push pair is
covered and the trade push pair is not, purely because nobody wrote the service method.

## 2. Decision: the canonical set is the code, and SPEC is the count cross-check

**The declared route set is the `client` const block. `docs/SPEC.md` is the number it
is checked against — not the other way round.**

Rule 5 says endpoint counts come only from `docs/SPEC.md`. That is satisfied here
*mechanically* rather than by deference: the guard asserts

```
len(declared consts) == the single total parsed from docs/SPEC.md
```

so SPEC and code cannot disagree without a red run, and the gate never depends on
anything but one regex over one ASCII line. The direction matters. If SPEC were the
source of the *set*, every check would inherit a Markdown table parser, and a table
reformat — a column width, a backtick, an em dash — would start failing CI. Rule 5
constrains where a *number* may be written down; it does not require a Markdown parser
to enumerate a set the compiler already knows.

**The residual weakness, stated plainly.** Rule 5 is a statement about discipline, and
this guard makes one number load-bearing rather than three. Today the number 51 is
written in four places: `docs/SPEC.md:122`, `client/routes_test.go:12`
(`wantRouteCount = 51`), `client/routes_e2e_test.go:41` (`!= 51`), and the eight `###
2.x` group headers. `client/routes_test.go:15` already fails if a route is added to
`client` without editing that literal — but it cannot detect SPEC drifting from code,
because its `51` is a Go literal that no Markdown edit touches. The guard adds exactly
that: SPEC's own total is now the expected number. A commit that adds a route to
`client` and SPEC but forgets `routes_test.go` is caught by `go test`; a commit that
adds a route to `client` and `routes_test.go` but **not** SPEC is caught by the guard
and by nothing else in the repository today.

What remains is the case the brief names: a single commit that hand-edits SPEC's
`51` → `52` *and* adds a const to `client` is coherent and must pass — it is a correct
commit. The undefeatable version is a commit that hand-edits SPEC's number *and* the
const block with no route added, which is only reachable by someone editing counts on
purpose. §5.2 makes the eight group headers and the `9 + 2 + …` breakdown load-bearing
too, so that edit has to be made in four coordinated places rather than one.

**What the guard is not.** It is not a second source of truth competing with
`client`. It reads `client` and reports.

## 3. Mechanism

**A standalone `package main` script at `scripts/paritygate/main.go`, invoked by a
Makefile target and its own CI job.** Not a Go test. The repository precedent is
`scripts/coverage_gate.go` → `make coverage` → the `coverage` job in
[`.github/workflows/ci.yml`](../../../.github/workflows/ci.yml), and
`internal/layering/layering_test.go` is the precedent for the *AST* technique. Both
precedents are kept; the question is only which shell the AST walk lives in.

### 3.1 The decisive reason: a passing Go test does not print its report

**C2 is report mode, and report mode's whole output is `t.Log`.** `go test` discards
`t.Log` output for a test that passes unless `-v` is passed. `go test ./...` in the
`build` job is not verbose, and neither is `go test -race -count=1 ./...`. A guard
written as a test in report mode would print nothing to anyone, ever, and the only
signal it could give is a green tick carrying no information — which is the exact
failure the brief asks to prevent. A script's `fmt.Println` always reaches the log.

The exit code is the second reason. Report mode must exit 0 *for a gap* and exit 1 for
a broken invariant; a test has one failure channel and no way to express "this is a
failure I am choosing not to report". Makefile recipes and CI steps speak exit codes.

### 3.2 The three costs of a test, checked rather than assumed

- **A test in `pkg/services` would perturb a gate B4 just closed.** `pkg/services` is
  at 100.0% and is registered in `scripts/coverage_gate.go`; a filesystem-walking test
  with unreachable branches would break a threshold C-task work paid for.
- **`pkg/services` is a C-task target.** C3–C13 edit it constantly. A guard living
  there is a merge hazard, not a feature.
- **A test's failure semantics are the wrong shape for a mode flip.** C14 is "flip
  report to enforce". In a test that is editing a `t.Fatalf`; in a script it is
  passing a flag. The flag makes the flip reviewable in one line of a command.

### 3.3 Why a subdirectory, not `scripts/parity_gate.go`

`scripts/` is a real package in this module: `go list ./...` prints
`github.com/shing1211/hstongapi4go/scripts`, and `go vet ./...` compiles it. A second
`package main` file with its own `func main()` in that directory collides with
`coverage_gate.go` and breaks `go build ./...` and `go vet ./...`. So the guard is
`scripts/paritygate/main.go`, run as `go run ./scripts/paritygate`.

`internal/layering/layering_test.go:65` skips any directory with the `scripts/`
prefix, so the new package is outside every existing boundary rule and needs no
layering exemption.

### 3.4 `go/parser` and `go/ast` only, and why the guard imports nothing

Stdlib only: rule 8 admits no new dependency without an ADR, and
`internal/layering/layering_test.go` already establishes that a hand-written
`go/parser` walk is the accepted way to assert a boundary in this repository — with a
GoDoc that explains exactly why the alternatives (golangci-lint `depguard`) were
rejected for looking like enforcement while enforcing nothing.

**The guard does not import `github.com/shing1211/hstongapi4go/client`, and this is a
deliberate departure from the task brief, which specified `len(client.Routes())`.**
The brief's version cannot be exercised against a fixture, because the registry would
come from the compiled binary rather than from the tree under test — and the brief's
own verification table (§7) is a table of mutations. A guard that can only be
mutation-tested by editing the repository produces weaker evidence and risks leaving a
planted mutation behind. The information is identical either way: `client.Routes()` is
a loop over the `canonicalRoutes` map, so parsing that map literal from
`client/routes.go` yields the same set. What the parse cannot see — that `Validate`
consults `canonicalRoutes` (`client/routes.go:184`) — is already pinned by
`client/routes_test.go:30-32` and `route_mutation_test.go`. The guard therefore has no
module coupling at all, so a compile error elsewhere in the tree can never be
misreported as a parity result, and every one of its checks is `--root`-fixture-able.

## 4. What counts as "implemented"

### 4.1 The primary signal, and why it is sound rather than heuristic

**Collect every selector expression `client.RouteXxx` in a non-test `.go` file under
the scan roots, where the receiver is the `*ast.Ident` named `client`, and `RouteXxx`
is a name in the declared set.**

`go/parser` + `go/ast` (`ast.Inspect` over `*ast.SelectorExpr`), no type checker.
`golang.org/x/tools/go/packages` would give a real type graph but is a new dependency;
it is not needed, because the soundness does not come from types:

1. `Executor.Do`'s third parameter is typed `client.Route`
   ([`pkg/services/executor.go:36`](../../../pkg/services/executor.go)), so a v-next
   call site **cannot** pass a bare string.
2. The only production implementation is `*client.Client`, asserted at
   `executor.go:43`, and it calls `route.Validate()`
   ([`client/client.go:149`](../../../client/client.go)) before sending.
3. `Validate` consults `canonicalRoutes`, so a `client.Do` call in production can only
   carry a registered route.
4. Therefore every v-next `Do` call names a registered route, and the ways to name one
   are exactly: its constant, a conversion `client.Route("…")`, or a value received
   from outside the scan roots.

Fact 3 in the brief reached (2)-(3) directly from `client.Do`. That is right but
incomplete, and the missing half matters: **`pkg/services` never calls `client.Do`.**
It calls `s.client.Do` through the `Executor` interface, where `s.client` is a field.
A guard written from the brief's phrasing alone might look for a `client.Do` call; the
AST must key on the route *argument*, matched as a package-qualified selector, and must
distinguish the package identifier `client` from the field selector `s.client`. Both
are `*ast.SelectorExpr`; only the route argument has an `*ast.Ident` receiver.

**The receiver test is also a self-check, and a fatal one.** If any scanned file
declares an identifier named `client` — a local, a parameter, a receiver, a
package-level `const`/`var`/`type`/`func` — then `client.RouteXxx` is ambiguous and
the walk would under-count silently. The guard must fail loudly in that case rather
than report a smaller gap.

> **Correction, found while implementing C2: the original claim that "there is no
> such declaration today" was wrong, in a way that would have made the guard
> unable to run.** Three struct fields are named `client`:
> `pkg/services/account.go:82`, `pkg/services/market.go:36`, and
> `pkg/services/trading.go:98`, all of type `Executor`. A naive receiver test that
> treated any `client` declaration as fatal would therefore refuse to run against
> the real repository. Fields are also **not** a shadowing risk: a field is reached
> only through its owner's selector (`s.client`), never as a bare `client`, so it
> cannot make `client.RouteXxx` ambiguous. The fatal check covers locals,
> parameters, receivers, package-level declarations, range clauses, type switches,
> and foreign imports of the `client` package; struct fields are excluded, and
> `TestStructFieldNamedClientIsNotAnError` pins that exclusion so it cannot be
> "fixed" back into a permanent failure.

### 4.2 The one blind spot, and the diagnostic that closes it

From (4) above: **an inline `client.Route("/hq/BasicQot")` conversion is a valid,
registered route that this scan cannot see**, so it would be simultaneously
*implemented* and reported *unwired* — the guard would be wrong in both directions at
once. The same holds for a route that reaches a `Do` call as a parameter typed
`client.Route` supplied by a caller outside the scan roots.

There are zero such conversions in `pkg/services` today. The only two in the
repository are `test/mockgateway/server.go:371,385`, which convert an inbound
`r.URL.Path` to dispatch on it — a Gateway-side switch, correctly outside the v-next
set.

The cheap secondary diagnostic, three parts, all over the same AST walk:

| Detect | Why it matters | Severity |
|--------|----------------|----------|
| `CallExpr` whose `Fun` is a conversion `client.Route(<string literal>)` | A registered route referenced without its named constant. Either the gap count is wrong or the reference is mis-credited; both are worth a line. | warning in report mode, **fatal** in enforcing |
| Any string literal in a scanned non-test file equal to a declared path, or to its `<path>Request` / `<path>RequestMsgType` alias | Catches the larger form — a service that builds the path itself and never mentions the constant. The alias forms are live: `client.NormalizePath` (`routes.go:214`) accepts all three. | same |
| A `client.Route`-typed parameter or struct field in a scanned package | The route can only be supplied from outside the scan roots, so no textual diagnostic can attribute it. Reported, never auto-resolved. | warning, always |

The third is the honest limit. It is listed because a guard that names its own blind
spots is useful and one that pretends to have none is not.

### 4.3 Scan roots

`pkg/services` is the primary root. It is joined by **`pkg/transport`** and
**`internal/auth`**, for a reason worth stating: `internal/auth` today takes an
injected `loginFn` and names no route at all
([`internal/auth/authenticator.go:25`](../../../internal/auth/authenticator.go)), so
C13's composition is the moment `RouteTradeLogin` first enters the v-next layer, and
C10's `PushOrchestration` is the moment `RouteTradeSubscribe` does. A `pkg/services`-only
scan that either of those lands outside would report a gap no task can close, and C14
would be unreachable. `pkg/domain` is excluded because
`internal/layering/layering_test.go:185-189` already forbids it from importing
`client`. Nothing else in the tree is in scope: `pkg/hstong/*` references all 51
between them and is deprecated, so including it would make the guard permanently green
and worthless.

The guard **prints the roots it scanned** and **prints `file:line` for every counted
reference**. 29 today, 51 at parity — small enough for a reviewer to audit, which is
the real defence against §4.4.

### 4.4 What the scan still cannot see

A `client.RouteXxx` that appears in code but is not passed to `Do` — a switch that
logs a route, a slice of routes for a batch helper — is credited as implemented. Making
the walk require a `Do` ancestor would fix it and break the ordinary `r :=
client.RouteX; go f(r)` shape, so the guard counts name references and shows its work.
That is the same trade `internal/layering/layering_test.go` makes: it enumerates
imports and asserts the walk found something, and the boundary is auditable by reading
it.

## 5. The three checks

`exit 1 ⟺ (check 1 or check 2 or an input-integrity failure) OR (--enforce AND gap > 0)`

That single sentence is the whole exit contract. Note what it says: **report mode never
fails *because of a gap*.** It may still fail because the route table is broken or the
guard could not read its inputs — and that is not a violation of "report mode does not
fail CI", because those conditions are all false today and none of C3–C13 can make
them true (§5.2).

### 5.1 Check 1 — the const block and the registry agree

Parse two composite literals out of `client/routes.go`: the `const` block of
`(name, path)` pairs, in declaration order, and the `var canonicalRoutes` map literal.
Assert both directions.

- A const path absent from `canonicalRoutes` → `client.Client.Do` would reject it:
  `errors.Is(route.Validate(), client.ErrUnknownRoute)`. No test covers this;
  `client/routes_test.go:24` iterates the *registry*, so a const omitted from it is
  invisible.
- A `canonicalRoutes` path with no const name → usable only by conversion, so the
  guard can never see a service reference it and it would be permanently unwired. That
  is the §4.2 blind spot promoted to a table error.
- The guard names the identifier it parsed and **errors if it does not find exactly one
  `canonicalRoutes` map literal**, so a rename or a split does not silently produce an
  empty registry.

```
ERROR parity: client/routes.go:30 RouteHqBasicQot = "/hq/BasicQot" is not in canonicalRoutes
       → client.Client.Do would refuse it: errors.Is(route.Validate(), client.ErrUnknownRoute)
ERROR parity: canonicalRoutes holds "/trade/Nope" with no constant in client/routes.go
       → no service can name it; add a Route constant or drop the entry
```

### 5.2 Check 2 — the declared count against SPEC

Read `docs/SPEC.md`, split on `\n`, `TrimSuffix` each line of `\r` (§5.4), and require
**exactly one** line matching `\*\*Total: (\d+) HTTP endpoints\*\*`. Compare that `N`
against the const count. Then, if the group-level check is adopted (recommended, see
below), require the eight `### 2.x` header counts to sum to `N` and to equal the
`(9 + 2 + 2 + 5 + 13 + 2 + 7 + 11)` breakdown on the same line.

```
ERROR parity: docs/SPEC.md:122 declares 52 HTTP endpoints; the client const block declares 51
ERROR parity: docs/SPEC.md group headers sum to 51 but line 122 breaks down as 50
ERROR parity: docs/SPEC.md declares no "**Total: N HTTP endpoints**" line
       → that line is the canonical count (AGENTS.md rule 5); a zero-match must never read as
         "0 endpoints, all clear"
```

**Zero matches is an error, never an empty set.** This is the single most important
invariant in the guard: a parser that silently returns nothing turns a rename into
"everything is wired".

**Group-level check: recommended, and here is how to do it without name matching.**
`docs/SPEC.md`'s group headers use an em dash — `### 2.8 Futures — 11`, U+2014,
verified — while `client/routes.go` comments use ASCII parentheses, `// Futures (11).`
Matching group *names* across those two conventions would break on a cosmetic edit, so
the guard must not. Instead it joins **positionally**: the SPEC route rows are numbered
1..51 in the same order as the const block (verified, 51/51, 0 mismatches), so group
boundaries come from the header counts and the k-th const belongs to the group the
first k headers cover. If the positional join finds a path disagreement at any
position, the guard **suppresses the group rendering and says so** rather than printing
a confidently wrong grouping.

### 5.3 Check 3 — the unwired set, grouped

`unwired = declared names − referenced names`, rendered by SPEC group, with the group
name, both counts, and the gap. The per-endpoint detail lists constant name and path.

**Grouping: yes. Task IDs: no.** A hand-typed `C3–C6` next to each group is a second
hand-maintained mapping, and AGENTS.md rule 5 exists precisely because hand-maintained
counts drift. `todos.md` is the owner column and it is one file away. §1.1 is what a
maintainer reads next to the report.

```
PARITY GUARD — REPORT MODE (informational; this run cannot fail on a coverage gap)
  v-next service coverage: 29/51 endpoints named by a service; 22 NOT implemented

  Group                        declared  wired  gap
  Market pull                          9      9    0
  Market subscription                   2      2    0
  Trade session                         2      0    2
  Trade assets / positions              5      5    0
  Trade orders                          13     13    0
  Trade push subscribe                  2      0    2
  Algo / strategy                       7      0    7
  Futures                              11      0   11
  TOTAL                                51     29   22

  Unwired (22):
    Trade session (2)
      RouteTradeLogin            /trade/TradeLogin
      RouteTradeLogout           /trade/TradeLogout
    …

  Scanned: pkg/services, pkg/transport, internal/auth (test files excluded)
  Referenced (29): pkg/services/account.go:116 RouteTradeQueryMarginFundInfo …

  PARITY: 29/51 gap=22 mode=report enforce=off
  A green exit in report mode does NOT mean v-next is at parity.
  exit=0
```

In enforcing mode the header becomes `PARITY GUARD — ENFORCING MODE`, the group table
is followed by every gap, and a non-zero gap exits 1.

### 5.4 Line endings: a live hazard on this dev host, measured

`docs/SPEC.md` in this working tree is **CRLF** — 993 CR bytes against 993 LF — while
`client/routes.go` is LF (0/229) and `pkg/services/market.go` is LF (0/663). So the
one file the guard parses as text is CRLF here and LF in CI, on the machine the run is
being executed on. `*.md text eol=lf` is in
[`.gitattributes`](../../../.gitattributes) but that only applies on checkout, and
`docs/adr/0010-*.md` is CRLF here for the same reason `proto/` was — the situation
[AGENTS.md](../../../AGENTS.md) already documents for `make proto-verify`.

A guard that splits on `\n` and matches a `$`-anchored pattern fails on this host and
passes on the runner, for a reason that has nothing to do with the code. So:

- Every line is `strings.TrimSuffix(line, "\r")` before matching, and the Total-line
  pattern is **not** `$`-anchored.
- SPEC lines contain non-ASCII (the em dash), so the parse is over `string` runes, not
  bytes.
- When a SPEC check fails, the offending line is printed with `%q` so a stray `\r` is
  visible in the message.
- The Go-side parse is unaffected: `go/parser` handles CRLF, and `gofmt -l` already
  guarantees the `.go` files are LF.

## 6. Modes

**`--enforce`, a flag, default off. Not an environment variable.** Three reasons. It
appears in the `run:` line of the CI step, so the mode is greppable in the workflow and
readable in the log; a `env:` block set once at job level is invisible there and is the
first thing lost in a copy-paste. Default-off makes the safe direction the default: a
bare `go run ./scripts/paritygate` is report mode, so a developer can never hit an
enforcing exit they did not ask for, and an exported shell variable cannot make a local
run enforcing by accident. And a flag is self-documenting in `make help`.

Two Makefile targets, both always present, so the mode is never a CI-only concept:

```
parity:         go run ./scripts/paritygate            # report, exit 0 on a gap
parity-enforce: go run ./scripts/paritygate --enforce  # exit 1 on any gap
```

**C14 changes `ci.yml`, not the Makefile.** The flip is the decision "may CI fail on a
SPEC/service mismatch", and that decision belongs where CI is configured. It is one
line in [`.github/workflows/ci.yml`](../../../.github/workflows/ci.yml) plus one line
in [`docs/RELEASE_CHECKLIST.md:13`](../../../docs/RELEASE_CHECKLIST.md), which lists
the jobs that must be green.

Report mode's output requirements, so a green run cannot be read as parity:

- The word `REPORT MODE` and the word `NOT` appear in the first two lines.
- The counts are phrased as a numerator/denominator with the gap stated as
  `22 NOT implemented` — never `29 endpoints wired ✓`, which reads as partial success.
- The words `PASS`, `OK`, `SUCCESS`, and `All endpoints` appear **only** when the gap
  is 0 and the mode is enforcing. A vocabulary ban is cheap and removes the whole
  class of misreading.
- The line `A green exit in report mode does NOT mean v-next is at parity.` is printed
  unconditionally in report mode.
- A final machine-readable line for grepping: `PARITY: 29/51 gap=22 mode=report
  enforce=off`.
- The exit code is echoed: `exit=0 (report mode never fails on a gap)`.

## 7. Integration

**Makefile:** `parity` and `parity-enforce` (§6), added to `.PHONY` and to the `make
help` list, and the corresponding rows in [AGENTS.md](../../../AGENTS.md)'s
`make help` block. `make` is absent on the dev host, so `go run ./scripts/paritygate`
must be the documented direct command and the recipe must be exactly that — the same
relationship `coverage_gate.go` has with `make coverage`. The guard must locate the repo
root from `runtime.Caller(0)` (which `go run` resolves to the real source path) with a
CWD fallback, and **fail loudly** if neither yields a `client/routes.go`; it must not
assume the CWD, because a silent wrong root produces a wrong report, not an error.

**CI:** a new job `parity` (display name `parity guard`) in
[`.github/workflows/ci.yml`](../../../.github/workflows/ci.yml), placed immediately
**before** the existing `coverage` job. It mirrors that job: `runs-on:
ubuntu-latest`, `actions/checkout@v7`, `actions/setup-go@v7` with `go-version-file:
go.mod` and `check-latest: true`, and a single `make parity` step. **No `needs:`** —
like `coverage`, it depends on nothing, so C14's flip cannot be masked by a green
`coverage` and a parity failure cannot be attributed to a test failure.

**Why a separate job and not a step in `build`.** Report mode exits 0 either way, but
a dedicated job gives parity its own log — which is where anyone asking "are we at
parity?" will look — its own place in the run's job list, and its own required status
at C14. Adding it to `build` would bury the report between the race-detector and
money-check output, and turning one step red in a seven-step job is a weaker signal
than a red job. The file already has 12 one-concern-per-job gates; a thirteenth is the
established shape.

**Before C14 it cannot redden CI.** Report mode exits 0 for any gap, and checks 1, 2
and the integrity conditions are all verified clean against the real tree today
(§5, §8.1). The only way to obtain a report-mode exit 1 is one of the enumerated
integrity failures, none of which C3–C13 can cause.

## 8. Failure modes and limitations

### 8.1 What the guard cannot see

- **An endpoint implemented outside the scan roots.** `pkg/hstong/*` references all 51
  collectively; a repo-wide scan would report 51/51 forever. The roots are explicit,
  printed, and never inferred.
- **A route threaded in from outside** (§4.2, third diagnostic). Unresolvable by any
  textual means; reported, never auto-resolved.
- **A route named but not used** (§4.4). Mitigated by printing `file:line` for every
  counted reference.
- **Whether the implementation is *correct*.** Referencing `RouteTradeFuturesEntrust`
  is what the guard checks; that the futures service validates its request, retries
  never, and decodes the reply is B3's and ADR 0003's business. `client/routes_e2e_test.go`
  already drives all 51 through `Do` with a generic param and a canned reply, so
  "reachable at the transport layer" and "implemented at the service layer" are two
  different questions and the guard answers only the second.
- **Topic and schema counts.** Rule 5 names them too. Not this guard — see §8.3.
- **Whether a group breakdown is *right*, only that it is *consistent*.** A commit that
  adds an algo endpoint and moves 7 → 8 passes every check. That is the correct
  outcome; the vendor decides how many algo endpoints exist.

### 8.2 Adding a route to `client` and SPEC in the same commit

Checks 1 and 2 pass at N+1. `client/routes_test.go:15` and
`client/routes_e2e_test.go:41` go **red** unless both literals are updated — the two Go
literals, not the guard, are the friction, and that friction is a good thing. The new
endpoint then appears immediately in the unwired list, which is correct: it is unwired
by definition until a service names it. In report mode: green with one more line. In
enforcing mode: red, until C3–C13 or a successor implements it. With the group check
adopted, the same commit must also bump the owning `### 2.x` header and the
`9 + 2 + …` breakdown — a correct requirement, since SPEC states both.

### 8.3 What the guard must not be extended to do

- **Not to topic or schema counts.** A second half-parsed source of truth is worse than
  none, and a different task should own it.
- **Not to `pkg/hstong/*`.** It references all 51 by design and is deprecated; including
  it makes the guard permanently green and therefore worthless.
- **Not to SPEC group *names*.** SPEC uses `—` (U+2014) and the const block uses `(9)`.
  Name-matching across those two conventions fails on a cosmetic edit.
- **Not to `pkg/services` coverage.** B5 already registered the package in
  `scripts/coverage_gate.go`; two gates on one package is how a threshold gets
  weakened.
- **Not to request shape, codec choice, or retry class.** ADR 0003 and B3 own those.
- **Not a second, weaker mode.** If the AST walk cannot be trusted the run should fail,
  not degrade to a regex.

### 8.4 The guard's own failure modes

| Condition | Required behaviour | Why |
|-----------|--------------------|-----|
| `canonicalRoutes` map literal not found, or found twice | error | otherwise the registry parse yields an empty set and every route reads as unwired |
| `**Total: N HTTP endpoints**` matched zero or twice | error | a zero-match must never read as "all clear" |
| any scan root missing | error | a renamed directory must not read as "nothing referenced" |
| zero references found | error | `internal/layering/layering_test.go:108-110` refuses to pass when its walk found nothing, for exactly this reason |
| an identifier named `client` declared in a scanned file | error | the walk would under-count silently (§4.1) |
| positional SPEC↔const join disagrees | suppress group rendering, say so | a confidently wrong grouping is worse than none |
| `docs/SPEC.md` absent | error | rule 5 makes it the only source of the count |
| repo root not resolvable | error | a wrong root produces a wrong report, not a failure |

## 9. Verification for C2

Run each case against a **throwaway tree** via `--root`, never by editing the
repository. `scripts/test_check_money.py` already sets this precedent — the Makefile
comment at line 93-96 records that it "exercises check_money.py against throwaway temp
trees and never plant[s] a float money field in the repository". The guard therefore
takes `--root DIR` defaulting to the resolved repo root, and **prints the resolved
paths it used**, so a fixture run is unmistakable in the log.

`--root` also requires the guard to parse `canonicalRoutes` rather than import
`client` (§3.4); with the import, V7 and V8 could not be run at all.

| # | Case | Setup | Expected | report | enforce |
|---|------|-------|----------|--------|---------|
| V1 | Full parity | fixture with all 51 named | groups all gap 0; `PARITY: 51/51 gap=0 mode=enforce` | 0 | 0 |
| V2 | **Current 29/51** | the real tree | the §5.3 table verbatim; `PARITY: 29/51 gap=22` | **0** | **1** |
| V3 | Route named only in a test file | fixture: `RouteTradeFuturesEntrust` in `pkg/services/x_test.go` only | still `gap=22`, Futures gap 11, detail lists the route | 0 | 1 |
| V4 | Inline-conversion blind spot | fixture: `client.Route("/trade/FuturesEntrust")` passed to `Do` in non-test code | `WARN parity: route referenced without its named constant: pkg/services/market.go:NN` **and** the route still counted unwired | 0 | 1 |
| V5 | SPEC/code count mismatch | fixture SPEC Total `51` → `52` | `ERROR parity: docs/SPEC.md:122 declares 52 HTTP endpoints; the client const block declares 51` | **1** | 1 |
| V6 | Group breakdown mismatch | fixture breakdown `… + 7 + 10` | `ERROR parity: group headers sum to 51 but line 122 breaks down as 50` | 1 | 1 |
| V7 | Const missing from registry | fixture: one const deleted from `canonicalRoutes` | `ERROR parity: client/routes.go:30 RouteHqBasicQot … is not in canonicalRoutes` | **1** | 1 |
| V8 | Registry entry with no const | fixture: bare path added to the map | `ERROR parity: canonicalRoutes holds "…" with no constant` | 1 | 1 |
| V9 | SPEC Total line removed | fixture: line 122 deleted | `ERROR parity: docs/SPEC.md declares no "**Total: N HTTP endpoints**" line` — and **must not** print `0 endpoints` or `all clear` | 1 | 1 |
| V10 | CRLF checkout | fixture SPEC written CRLF | byte-identical output to the LF fixture; no `$`-anchored parse failure | 0 | 0 |
| V11 | Shadowed `client` identifier | fixture: `client := 1` in a scanned non-test file | `ERROR parity: pkg/services/market.go:NN declares identifier "client"; the reference walk would under-count` | 1 | 1 |
| V12 | Missing scan root | fixture without `pkg/services/` | `ERROR parity: scan root pkg/services does not exist` — and **must not** print 0 referenced | 1 | 1 |

### 9.1 Negative control: the guard must be shown to fail

**A guard that cannot fail is not a guard.** V1 passing proves nothing on its own, and
C2 must not record "exit 0" for V1 as its evidence. Three observations are mandatory,
and all three must appear literally in the `todos.md` Evidence cell with their output
lines and exit codes:

1. **V2 in enforcing mode exits 1** and prints all 22 gaps. This is the control for the
   gap check itself — the thing C14 turns on.
2. **V5 in *report* mode exits 1.** This is the control for the report/enforce
   boundary: it proves report mode is not "never fails" but "never fails *on a gap*",
   which is the distinction §6 rests on.
3. **V7 in report mode exits 1.** This is the control for the registry check, and the
   only one of the three that reddens CI before C14 — which is correct, because a const
   missing from `canonicalRoutes` is a broken route table, not a parity gap.

The repository has done this deliberately twice and the pattern should be named in the
evidence: B5 raised the coverage threshold to 100.1 to prove the gate could fail, and
`internal/layering/layering_test.go:108-110` refuses to pass when its walk found
nothing. The additional requirement specific to this guard: after every V3–V12 fixture
run, `git status --short` must show no modification to the repository, which is the
check that the fixture-tree discipline actually held.

`gofmt -l .` and `go vet ./...` must stay clean with the new package present, and
`addlicense -check scripts/` must find its SPDX header — `scripts/` is already in the
`license`/`license-check` recipe's directory list, so this is a consequence of §3.3
rather than a new registration.

## 10. Rejected alternatives

| Alternative | Why rejected |
|-------------|--------------|
| A Go test in `pkg/services` | Its report-mode output is invisible without `-v` (§3.1), it perturbs the 100% gate B4 closed, and C3–C13 edit that package constantly. |
| A Go test in a new `internal/parity` package | Solves the coverage and merge-hazard problems, not the invisible-output one. Report mode still needs a flag, and a flag on `go test` is a lie. |
| `SPEC.md` as the source of the *set* | Every check inherits a Markdown table parser, and a reformat breaks CI. Rule 5 constrains where a number is written, not how a set is enumerated. |
| Import `client` and call `Routes()` | Identical information, but V7 and V8 become untestable without editing the repository, and a compile error elsewhere becomes a parity result. Rejected in §3.4. |
| `HSTONG_PARITY_ENFORCE=1` | Invisible in the CI step's `run:` line, lost in copy-paste, and an exported variable can make a local run enforcing by accident. |
| Default enforcing, `--report` to opt out | Every existing invocation — a developer's `make parity`, an existing CI step — would redden on day one, which is exactly what C2 exists to prevent. |
| Regex or `grep` over `pkg/services` | `pkg/services` already contains `client.Do`, `client.JSON`, `client.Route`, and `client.Codec` in `executor.go`, in comments and in the interface signature. A `client\.Route` pattern matches all of them, and a prefix match on `client.RouteHq` also matches `client.Routes`. |
| Scan `_test.go` too | Verified: today the test files name the same 29, so it would look harmless — and then the guard's own test, which must name all 51, and any TDD test written before its implementation, would both count as implementations. The exclusion is load-bearing, not cosmetic. |
| Print the owning task ID next to each group | A second hand-maintained mapping, in the shape rule 5 exists to forbid. `todos.md` is the owner column. |
| Extend to topic and schema counts | A different guard, against different sources. One half-parsed source of truth is worse than two honest ones. |

## 11. Not done here

No Go file was changed. The deliverable is this note. C2 implements
`scripts/paritygate/main.go`, `make parity` / `make parity-enforce`, and the `parity`
CI job; C14 flips the job's command and adds it to the release checklist's job list.

Two items in the run's own bookkeeping, found while verifying this design and not
acted on:

- **The labels `V1`–`V8` in [todos.md](./todos.md) are defined nowhere in the
  repository.** A grep for them returns only `todos.md`; neither
  [plan.md](./plan.md) nor [docs/DESIGN.md](../../DESIGN.md) defines them. C2's
  acceptance reads "V3b passes", so either the plan owner defines the labels or §9
  above becomes the definition of V3b.
- **Two of the 22 unwired endpoints are owned by no task** (§1.1). C10's design note
  should state whether `PushOrchestration` owns `/trade/TradeSubscribe` and
  `/trade/TradeUnsubscribe`, or a task must be added before C14 is reachable.
