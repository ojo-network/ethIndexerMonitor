build:
	go build -o ./build/ ./...

start:
	${MAKE} build
	./build/ethIndexerMonitor ./config.toml

.PHONY: build start