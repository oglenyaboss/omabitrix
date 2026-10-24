BINS = bin/omabitrixd bin/omabitrix

build: $(BINS)

bin/omabitrixd:
	go build -o $@ ./cmd/omabitrixd

bin/omabitrix:
	go build -o $@ ./cmd/omabitrix

test:
	go vet ./... && go test ./...

install: build
	install -m 755 bin/omabitrixd bin/omabitrix ~/.local/bin/

.PHONY: build test install
