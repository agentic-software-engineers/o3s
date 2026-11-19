# Contributing

## Changes

- **Branches** ideally start from an issue, carry one change each, and land on main through a pull request as one squashed commit.
- **Commits** start with the kind of change and a scope from `.github/tools/commitlint.json`, followed by a short summary.
- **Descriptions** of pull requests and issues stay slim and minimal, about 2–3 sentences on average.
- **Updates** of dependencies arrive from the Renovate bot, one pull request per update.

## Code

- **Comments** state a step's intent, in abstract terms, so they stay accurate as the concrete details change. Every branch and loop carries one, on the line above it.
- **Tests** come first, so a change starts with a test that fails without it, and the implementation then makes it pass.

## Releases

- **Versions** are left to the release-please bot, whose pull request raises the first number for breaking changes, the second for new features, and the third for fixes.
- **Releases** go out when that pull request merges.
- **Images** reach ghcr.io with every release, tagged with their version.

## Tests

- **Layers** run from many unit tests next to their code, through integration tests in `test/integration` that raise one component, to a few e2e tests in `test/e2e` that raise the whole stack, one package per isolation (`process`, `vm`), each layer behind its build tag, and a thin layer of scripts runs the binary in a pseudo-terminal for what only a real terminal shows.
- **Names** state the behavior under test, a comment above each test repeats it as a sentence, and fixtures take their values from `internal/fixture`, which keeps to the names RFC 2606 and RFC 5737 reserve for examples, the same value wherever a fixture plays the same part.
- **Bodies** check one behavior each, in setup, action and check set apart by blank lines.
- **Checks** use the standard library, go-cmp and golden files, compare whole values, test errors by identity, and report what they got against what they wanted.
- **Helpers** set up a named state and undo it once the test ends, and fakes live where they are used, held to the real adapter by a contract test, or in a helper package named for what it tests where several packages share them.
- **Menu** paths are walked until every answer of every question is taken once, each driven on the screen, and every screen and plan is held to the menu's rules, today's breaks named until a fix removes them, with seeded random keys from the screens of every path, the same on every run.
- **Goldens** refresh with the update flag, and a change reviews them as diffs.
- **Walks** of the paths a change touches, at a real terminal, come before its review.
