# minictd — GĐ2 control-plane daemon (Go stdlib) + minict-exec (C)
#
#   make build    — go build -> bin/minictd + bin/sandbox (CLI, issue #91)
#   make c-build  — cc minict-exec -> bin/minict-exec
#   make vet      — go vet ./... + gofmt check
#   make selftest — minict-exec --selftest-seccomp
#   make all      — build + c-build + vet + selftest
#
# The MINICT_SPEC patch that used to live in patches/ is now merged into
# sandbox/minict.c (D23/D24), so the old runtime-check target is gone.

GO  ?= go
CC  ?= cc
BIN ?= bin

.PHONY: all build c-build vet selftest clean

all: build c-build vet selftest

build:
	mkdir -p $(BIN)
	$(GO) build -o $(BIN)/minictd ./cmd/minictd
	$(GO) build -o $(BIN)/sandbox ./cmd/sandbox

c-build:
	mkdir -p $(BIN)
	$(CC) -O2 -Wall -Wextra -o $(BIN)/minict-exec cmd/minict-exec/minict-exec.c

vet:
	$(GO) vet ./...
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }

selftest: c-build
	./$(BIN)/minict-exec --selftest-seccomp

clean:
	rm -f $(BIN)/minictd $(BIN)/minict-exec $(BIN)/sandbox
