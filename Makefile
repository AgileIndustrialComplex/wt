.PHONY: build test clean

build:
	go build -o wt ./cmd/wt

test:
	go test ./...

clean:
	rm -f wt
