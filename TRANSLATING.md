# Translating

Thanks for helping translate `hstongapi4go`!

## Canonical language

**English is authoritative.** Every README translation links back to
[`README.md`](./README.md). If a translation and the English version disagree,
the English version is correct. The canonical README is also the clock for the
sync banner: its `Last synced:` date is the one every translation must match.

## Supported languages

| Locale | Language | File |
|--------|----------|------|
| `en` | English (canonical) | [`README.md`](./README.md) |
| `zh-Hans` | 简体中文 | [`README.zh-Hans.md`](./README.zh-Hans.md) |
| `zh-Hant` | 繁體中文 | [`README.zh-Hant.md`](./README.zh-Hant.md) |
| `ja` | 日本語 | [`README.ja.md`](./README.ja.md) |
| `ko` | 한국어 | [`README.ko.md`](./README.ko.md) |
| `es` | Español | [`README.es.md`](./README.es.md) |

Use BCP-47 tags, preferring script tags for Chinese (`zh-Hans` / `zh-Hant`).

## Scope

- **Translated:** `README.md` only.
- **Not translated:** `LICENSE`, `NOTICE`, `DISCLAIMER.md`, `SECURITY.md`,
  `CONTRIBUTING.md`, and the `docs/` set. English remains canonical for these.
  Do not translate legal text.

## Lockstep rule

All six READMEs move together. The same change — a new section, a changed
count, a new command — is applied to the English README and to every translation
in a single pull request. Nothing is allowed to sit half-translated.

Two things keep the six files visibly in lockstep, and
[`scripts/check_i18n.py`](./scripts/check_i18n.py) enforces both:

1. **Language switcher.** A single line, identical in every README, linking all
   six files in the fixed order:

   ```markdown
   [English](./README.md) · [简体中文](./README.zh-Hans.md) · [繁體中文](./README.zh-Hant.md) · [日本語](./README.ja.md) · [한국어](./README.ko.md) · [Español](./README.es.md)
   ```

2. **Sync banner.** A `Last synced:` line whose date equals the canonical
   README's date:

   ```markdown
   > Last synced: 2026-09-21
   ```

   Translations carry the banner in the local language, for example:

   ```markdown
   > 本文件是英文 [README](./README.md) 的社区翻译。**英文版本为准。**
   > 同步于 / Last synced: 2026-09-21
   ```

## Adding a language

1. Copy `README.md` to `README.<locale>.md`.
2. Add the language to the switcher line in **every** `README*.md` file, in the
   canonical order listed above. The switcher must be byte-for-byte identical in
   all six files.
3. Add the translation banner (below the switcher), matching the canonical
   `Last synced:` date.
4. Add the locale to `LANGUAGES` and `NATIVE_CONTENT` in
   [`scripts/check_i18n.py`](./scripts/check_i18n.py), then run the check and fix
   anything it reports.
5. Update the supported-languages table in this file.
6. Open a pull request.

## Keeping in sync

When the English README changes materially:

- Update the affected section in every translation.
- Bump the `Last synced:` date in the canonical README **and** in every
  translation to the same value. `scripts/check_i18n.py` fails if any file's
  date differs from the canonical README.
- Run the check:

  ```sh
  python scripts/check_i18n.py
  ```

  (On systems where `python` is not Python 3, use `python3`. `make docs-check`
  runs the same check alongside the link check and the strict MkDocs build.)

## Rules

- Do **not** hand-edit numbers (endpoints, topics, schemas, versions). They come
  from [`docs/SPEC.md`](./docs/SPEC.md); copy the current values
  (`51` endpoints, `11` push topics, `17` protos, Gateway `v2.4.1`, protobuf
  `v2.2.0`), do not re-derive them.
- Keep code blocks, commands, environment-variable names, file paths, route
  paths, JSON keys, identifiers, and URLs identical to English. Only comments
  inside code samples are translated.
- Keep the switcher line and the `Last synced:` banner format exact —
  `scripts/check_i18n.py` enforces both and also fails on structural drift (a
  heading count more than a small tolerance away from English) or on a file with
  no native-language content.
- Translation quality over quantity: a wrong translation of a trading SDK can
  cause real harm.
