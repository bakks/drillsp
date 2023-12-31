
gofiles := $(shell find . -name '*.go' -type f -not -path "./vendor/*")
buildtime := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
commit := $(shell bash ./bin/gitcommit.sh)
flags := -X main.BuildVersion=dev -X main.BuildArch=dev -X main.BuildCommit=${commit} -X main.BuildTimestamp=${buildtime} -X main.BuildOs=dev

all: build

bin/drillsp: $(gofiles) Makefile go.mod go.sum
	mkdir -p bin
	go build -ldflags "${flags}" -o ./bin/drillsp

clean:
	rm -f bin/drillsp proto/*.go

watch: Makefile
	find . -name "*.go" -o -name "*.proto" | entr -c make

build: bin/drillsp

licenses:
	go-licenses report ./... 2>/dev/null | awk -F"," '{printf "|[%s](https://%s)|[%s](%s)|\n",$$1,$$1,$$3,$$2}'


.PHONY: all clean watch test build licenses

