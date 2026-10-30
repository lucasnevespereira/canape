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
