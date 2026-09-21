# Thank you for contributing!

Please fill out the information below before submitting your pull request.
DCO sign-off is required for every commit — use `git commit -s` to sign your work.

---

## Description

<!-- What does this PR do? Briefly summarize the changes. -->

## Motivation

<!-- Why is this change needed? Reference an open issue with "Closes #N" or "Fixes #N" if applicable. -->

## Checklist

- [ ] Tests added or updated for the changes
- [ ] `make check` passes locally (gofmt, vet, money-check, unit tests)
- [ ] `gofmt -l .` prints nothing (no new unformatted files)
- [ ] No `float32`/`float64` fields added to money or quantity struct fields
- [ ] No changes to generated code under `gen/` (regenerate with `make proto` if needed)
- [ ] Documentation updated if public API or behaviour changed
- [ ] Commits are signed off (`git commit -s`) with DCO

---

Signed-off-by: Your Name <you@example.com>
