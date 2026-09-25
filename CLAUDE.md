<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **hstongapi4go** (6,382 symbols, 17,083 relationships, 518 execution flows; index built at `b30b0bf`).

The index lives in `.gitnexus/` and is **not committed** — it is a local artifact, so a fresh clone has none.

> No index yet? Run `gitnexus analyze --index-only` from the project root; the CLI is installed globally. If `.gitnexus/run.cjs` exists, `node .gitnexus/run.cjs analyze --index-only` works too. Otherwise bootstrap with `npx`, `bunx`, or `pnpm dlx` — e.g. `bunx gitnexus@latest analyze` (npm 11 npx crash; #1939).
>
> `Database file version: N, Current build storage version: M`? The MCP server process is older than the installed CLI. Restart the MCP server or the editor — **do not** re-run `analyze`; the index is healthy.

## Always Do

- **MUST run impact before editing a function, class, or method, whenever the graph can answer.** Use `impact({target: "symbolName", direction: "upstream"})` or `node .gitnexus/run.cjs impact "symbolName" --direction upstream --repo .`; report callers, processes, and risk. Text search is not a substitute while the graph answers.
- **MUST analyze graph changes before committing, whenever the graph can answer.** Use `detect_changes({scope: "all"})` (MCP) or `node .gitnexus/run.cjs detect-changes --scope all --repo .` (CLI fallback). `partial: true` or `truncated: true` is not a clean check — a zero means unseen, not unaffected; re-run it. For regression review: `detect_changes({scope: "compare", base_ref: "main"})` or `node .gitnexus/run.cjs detect-changes --scope compare --base-ref "main" --repo .`.
- **If the graph cannot answer — no index on a fresh clone, or a failing graph reader — say so, fall back to text search plus tests, and record in your summary that graph analysis was unavailable.** Never skip the step silently, and never rebuild a working index to satisfy it.
- MUST warn on HIGH/CRITICAL `risk` pre-edit; never use `riskSharedAxes` to waive a HIGH/CRITICAL `risk` warning. Compare File/symbol: MCP File omits axes; Graph-RAG expands File.
- **MUST treat `risk: UNKNOWN` as unresolved, not as low.** An empty caller set is not evidence the symbol is unused — it can also mean the callers are not resolvable by the index (plain-object property access, dynamic dispatch, cross-language calls). `impact` pairs `UNKNOWN` with a `riskNote` saying so. Confirm with a text search before treating the symbol as safe to change or delete; do not proceed on the strength of a zero.
- **MUST use `query({search_query: "concept"})` for concepts/flows, `context({name: "symbolName"})` for a named symbol, or `impact` for blast radius, on read-only callers, dependencies, imports, or execution flow.** Graph first; text search only for empty/`UNKNOWN`/literals.
- For security review, `explain({target: "fileOrSymbol"})` lists taint findings (source→sink flows; needs `analyze --pdg`).

## Never Do

- NEVER edit a function, class, or method before MCP/CLI impact analysis, when the graph can answer.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis, and never read `UNKNOWN` as an all-clear — it means the walk could not answer, which is the one verdict that requires confirming by other means.
- NEVER rename symbols with find-and-replace — use `rename` which understands the call graph.
- NEVER commit before MCP/CLI graph change analysis, when the graph can answer.

## Resources

| Resource | Use for |
| --- | --- |
| `gitnexus://repo/hstongapi4go/context` | Codebase overview, check index freshness |
| `gitnexus://repo/hstongapi4go/clusters` | All functional areas |
| `gitnexus://repo/hstongapi4go/processes` | All execution flows |
| `gitnexus://repo/hstongapi4go/process/{name}` | Step-by-step execution trace |

The `gitnexus://` URIs need a live MCP server. With the CLI alone, the equivalents are `gitnexus cypher` (structural queries), `gitnexus query`, and `gitnexus context`.

## CLI

| Task | Read this skill file |
| --- | --- |
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->
