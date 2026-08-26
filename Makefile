.PHONY: web bin test run release

web:
	cd web && npm install && npm run build

bin: web
	go build -ldflags "-X main.version=dev" -o bin/hallo ./cmd/hallo

test:
	go test ./...

run: bin
	HALLO_DEV=1 ./bin/hallo serve --listen :18080 --data data

release:
	sh scripts/release.sh $(v)
