.PHONY: scan serve test

scan:
	go run . scan

serve:
	go run . serve

test:
	go vet ./...
	go test ./...
