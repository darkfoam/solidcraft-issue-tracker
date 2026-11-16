IMAGE ?= localhost/solidcraft-triage:latest
TRIAGE_DB ?= ../data/triage.sqlite

.PHONY: build test fmt image backup clean

build:
	go build -o bin/triage ./cmd/triage

test:
	go test ./...

fmt:
	gofmt -w .

image:
	podman build -t $(IMAGE) .

backup:
	podman volume export solidcraft-triage-data -o solidcraft-triage-backup.tar

clean:
	rm -f bin/triage
