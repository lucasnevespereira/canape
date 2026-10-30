.PHONY: scan serve test release

scan:
	go run . scan

serve:
	go run . serve

test:
	go vet ./...
	go test ./...

# make release v=0.1.0 tags main and pushes the tag. CI then builds the
# binaries, publishes the GitHub release and updates the Homebrew formula.
release:
	@test -n "$(v)" || (echo "usage: make release v=0.1.0" && exit 1)
	@test "$$(git branch --show-current)" = main || (echo "release from main" && exit 1)
	@test -z "$$(git status --porcelain)" || (echo "commit or stash your changes first" && exit 1)
	git pull --ff-only
	go vet ./...
	go test ./...
	git tag -a v$(v) -m "canape v$(v)"
	git push origin v$(v)
