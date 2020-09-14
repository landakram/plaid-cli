build:
	go build -o bin/plaid-cli

release:
	goreleaser --rm-dist
