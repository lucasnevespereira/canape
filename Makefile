.PHONY: scan serve

scan:
	go run ./cmd/scan

serve:
	go run . serve
