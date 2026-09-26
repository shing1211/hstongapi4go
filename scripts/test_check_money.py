# Copyright 2026 shing1211
# SPDX-License-Identifier: Apache-2.0
"""Tests for check_money.py — the ADR 0008 money/quantity float guard.

Run from the repository root:

    python -m unittest discover -s scripts -p "test_*.py" -v

stdlib unittest is the only option: AGENTS.md rule 8 admits no new dependency
without an ADR, and the repo has no Python test framework installed.  There was
no prior Python test convention to follow (check_links.py and check_i18n.py carry
no tests either), so the standard-library layout — a ``test_*.py`` beside the
module, discovered by name — is what this establishes.

The end-to-end cases drive ``main()`` against a throwaway ``pkg/`` tree in a
temp directory, so no float money field is ever planted in the repository.  The
case that motivated the json-tag rule is ``TickSize float64
`json:"spreadLevel"``: its Go name tokenises to {Tick, Size} and is innocent,
while its wire key tokenises to {spread, Level} and is a price spread.  Judging
the Go name alone let that float through; these tests pin both directions so the
rule can neither miss it again nor fire on every float in pkg/.
"""

import contextlib
import io
import os
import shutil
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import check_money  # noqa: E402  (path must be set first)


# The exact field that escaped the guard, and a float that is genuinely not
# money on either name.  Both are planted in temp trees, never in the repo.
PLANTED = '\tTickSize float64 `json:"spreadLevel"`\n'
INNOCENT = '\tCount float64 `json:"queryCount"`\n'


class MoneyGuardTest(unittest.TestCase):
    """Base class providing a temporary pkg/ tree and a main() driver."""

    def setUp(self):
        self.root = tempfile.mkdtemp(prefix="check_money_test_")
        self.addCleanup(shutil.rmtree, self.root, ignore_errors=True)
        self.pkg = os.path.join(self.root, "pkg")

    def plant(self, decl, subdir="planted", filename="planted.go"):
        """Write a one-field struct into a temp pkg/<subdir>/<file> and return it."""
        d = os.path.join(self.pkg, subdir)
        os.makedirs(d, exist_ok=True)
        path = os.path.join(d, filename)
        with open(path, "w", encoding="utf-8") as fh:
            fh.write(
                "// Copyright 2026 shing1211\n"
                "// SPDX-License-Identifier: Apache-2.0\n"
                "package %s\n"
                "\n"
                "type orderBook struct {\n%s}\n" % (subdir, decl)
            )
        return path

    def run_guard(self):
        """Run main() against the temp tree; return (exit code, stdout, stderr)."""
        out, err = io.StringIO(), io.StringIO()
        saved = check_money.PKG_DIR, check_money.ROOT
        check_money.PKG_DIR, check_money.ROOT = self.pkg, self.root
        try:
            with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
                code = check_money.main()
        finally:
            check_money.PKG_DIR, check_money.ROOT = saved
        return code, out.getvalue(), err.getvalue()


class TestWireKeyRule(MoneyGuardTest):
    """The planted case: a monetary wire key behind an innocuous Go name."""

    def test_go_name_alone_would_pass(self):
        """Guard the premise: {Tick, Size} carries no money token."""
        self.assertFalse(check_money.is_money_name("TickSize"))
        self.assertTrue(check_money.is_money_name("spreadLevel"))

    def test_planted_spread_level_float_fails(self):
        """A float tagged spreadLevel must be reported, naming field and key."""
        self.plant(PLANTED)
        code, _out, err = self.run_guard()

        self.assertEqual(code, 1, "guard must fail on the planted case")
        self.assertIn("pkg/planted/planted.go:6:", err)
        # Both the Go field and the wire key are named, so the report is actionable.
        self.assertIn('TickSize float64 `json:"spreadLevel"`', err)
        self.assertIn('wire key "spreadLevel"', err)
        self.assertIn("matched on wire key", err)

    def test_negative_case_still_passes(self):
        """Count/queryCount is money on neither name: the rule must not blanket-fire."""
        self.plant(INNOCENT)
        code, out, err = self.run_guard()

        self.assertEqual(code, 0, "guard must not fail: %s" % err)
        self.assertIn("money-check OK", out)
        self.assertEqual(err, "")

    def test_innocent_float_alongside_planted_float(self):
        """Only the offending field is reported when both are present."""
        self.plant(PLANTED + INNOCENT)
        code, _out, err = self.run_guard()

        self.assertEqual(code, 1)
        self.assertEqual(len(err.strip().splitlines()), 3)  # header + 1 + hint
        self.assertNotIn("queryCount", err)

    def test_wire_key_with_tag_options(self):
        """json:"spreadLevel,omitempty" is the same wire key."""
        self.plant('\tTickSize float64 `json:"spreadLevel,omitempty"`\n')
        code, _out, err = self.run_guard()

        self.assertEqual(code, 1)
        self.assertIn('wire key "spreadLevel"', err)

    def test_snake_case_wire_key(self):
        """Wire keys are not always camelCase; tokenising must not assume so."""
        self.plant('\tTickSize float64 `json:"spread_level"`\n')
        code, _out, err = self.run_guard()

        self.assertEqual(code, 1)
        self.assertIn('wire key "spread_level"', err)


class TestGoNameRuleUnchanged(MoneyGuardTest):
    """The pre-existing Go-name rule must keep working exactly as before."""

    def test_money_go_name_with_json_tag(self):
        self.plant('\tLastPrice float64 `json:"last"`\n')
        code, _out, err = self.run_guard()

        self.assertEqual(code, 1)
        self.assertIn("matched on field name", err)
        self.assertIn('wire key "last"', err)

    def test_money_go_name_without_json_tag(self):
        """No tag at all: still judged on the Go name alone."""
        self.plant("\tLastPrice float64\n")
        code, _out, err = self.run_guard()

        self.assertEqual(code, 1)
        self.assertIn("no json tag", err)
        self.assertIn("matched on field name", err)

    def test_non_float_money_field_passes(self):
        """Only floats are rejected; string and json.Number stay legal."""
        self.plant(
            '\tLastPrice string `json:"lastPrice"`\n'
            '\tTickSize json.Number `json:"spreadLevel"`\n'
            '\tLastClosePrice []string `json:"lastClosePrice"`\n'
        )
        code, _out, err = self.run_guard()

        self.assertEqual(code, 0, "guard must not fail: %s" % err)

    def test_test_files_are_excluded(self):
        """The docstring promises tests are skipped; a float in one must not fail."""
        self.plant(PLANTED, filename="planted_test.go")
        code, _out, err = self.run_guard()

        self.assertEqual(code, 0, "guard must not fail: %s" % err)

    def test_missing_pkg_dir_is_not_an_error(self):
        """`make check` must stay green while pkg/ is still a skeleton."""
        out = io.StringIO()
        saved = check_money.PKG_DIR
        check_money.PKG_DIR = os.path.join(self.root, "absent")
        try:
            with contextlib.redirect_stdout(out):
                code = check_money.main()
        finally:
            check_money.PKG_DIR = saved

        self.assertEqual(code, 0)
        self.assertIn("does not exist yet", out.getvalue())


class TestMalformedTags(MoneyGuardTest):
    """A missing or odd tag is judged on the Go name, never a crash."""

    def test_tag_without_a_json_key(self):
        self.plant('\tLastPrice float64 `db:"last"`\n')
        code, _out, err = self.run_guard()

        self.assertEqual(code, 1)
        self.assertIn("no json tag", err)
        self.assertIn("matched on field name", err)

    def test_unterminated_backtick(self):
        """A malformed literal yields no wire key; the Go name still decides."""
        self.plant('\tTickSize float64 `json:"spreadLevel"\n')
        code, _out, err = self.run_guard()

        self.assertEqual(code, 0, "guard must not fail: %s" % err)

    def test_json_dash_is_not_money(self):
        """json:"-" is not a wire key and tokenises to nothing; Go name decides."""
        self.plant('\tTickSize float64 `json:"-"`\n')
        code, _out, err = self.run_guard()

        self.assertEqual(code, 0, "guard must not fail: %s" % err)

    def test_commented_out_field_is_ignored(self):
        self.plant("\t// TickSize float64 `json:\"spreadLevel\"`\n")
        code, _out, err = self.run_guard()

        self.assertEqual(code, 0, "guard must not fail: %s" % err)


class TestWireKeyUnit(unittest.TestCase):
    """Unit coverage for the tag parser itself."""

    def test_parses_name_only(self):
        self.assertEqual(check_money.wire_key('`json:"spreadLevel"`'), "spreadLevel")

    def test_strips_options(self):
        self.assertEqual(
            check_money.wire_key('`json:"spreadLevel,omitempty" xml:"s"`'),
            "spreadLevel",
        )

    def test_absent_json_key(self):
        for tag in (None, "", "``", '`db:"last"`', '`yaml:"last"`', '`json:"a" `json:"b"`'):
            with self.subTest(tag=tag):
                got = check_money.wire_key(tag)
                if tag == '`json:"a" `json:"b"`':
                    # The first json key in the span wins, matching the reader.
                    self.assertEqual(got, "a")
                else:
                    self.assertIsNone(got)

    def test_empty_name_is_not_a_key(self):
        self.assertIsNone(check_money.wire_key('`json:" "`'))
        self.assertIsNone(check_money.wire_key('`json:",omitempty"`'))


if __name__ == "__main__":
    unittest.main()
