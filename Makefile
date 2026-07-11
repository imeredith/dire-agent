.PHONY: production

production:
	npm --prefix web run build:embedded
	mkdir -p dist
	go build -tags webui -trimpath -o dist/dire-agentd ./cmd/dire-agentd
