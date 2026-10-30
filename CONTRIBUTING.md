# Contributing

Thanks for helping. canape is small on purpose: a few packages, few dependencies, one binary.

## Setup

You need Go (see `go.mod` for the version) and a machine on the same network as the devices you're testing.

```bash
go run . help      # any command works with go run .
make test          # go vet and go test, like CI
```

`internal/` is described in [AGENTS.md](AGENTS.md#layout), along with the design decisions. Coding agents read the same file.

## Adding a TV brand

The most useful contribution. Follow [docs/adding-a-tv.md](docs/adding-a-tv.md), with the TV in front of you.

## Pull requests

- Keep them small: one change you can explain in a sentence.
- `gofmt`, `go vet` and `go test` must pass. CI runs them.
- Commit messages are one line, Conventional Commits style: `feat(tv): add Samsung driver`.
- A new dependency needs a reason in the PR description: canape prefers the standard library.
- No real IPs, MAC addresses, Wi-Fi names, passwords or tokens in code, tests or docs. Use `192.168.1.20`, `aa:bb:cc:dd:ee:ff` and the like.

## Releasing

Versions follow [SemVer](https://semver.org). While canape is at 0.x, a minor bump (0.2.0) means new features or changed commands, and a patch bump (0.1.1) means fixes only.

```bash
make release v=0.2.0
```

That checks you're on a clean, up to date `main`, runs the tests, tags `v0.2.0` and pushes the tag. CI does the rest: binaries for macOS, Linux and Raspberry Pi, the GitHub release, and the Homebrew formula. Release notes are generated from the commit messages, which is one more reason to keep them clear.

Pre-releases work too: `make release v=0.2.0-beta.1` publishes binaries and leaves Homebrew alone.

The Homebrew step needs a `HOMEBREW_TAP_TOKEN` secret on this repo: a GitHub token that can push to `lucasnevespereira/homebrew-tools`.

If that step fails, re-run it alone for the tag: `gh workflow run release.yml -f tag=v0.2.0`. As a last resort, set `url` and `sha256` in `canape.rb` in the tap by hand. The checksum is `curl -sL <url> | shasum -a 256`.
