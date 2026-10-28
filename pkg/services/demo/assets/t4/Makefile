CC ?= gcc
CFLAGS = -Wall -Wextra -Werror -pedantic -std=c17 -O2 -Isrc

.PHONY: build test format lint e2e clean

build:
	mkdir -p bin
	$(CC) $(CFLAGS) -o bin/t4 src/t4.c src/http.c

test:
	mkdir -p bin
	$(CC) $(CFLAGS) -Itests/unit -o bin/test_http src/http.c tests/unit/test_http.c
	./bin/test_http

format:
	@clang-format -i src/*.c src/*.h tests/unit/*.c 2>/dev/null || true

lint:
	@clang-tidy src/*.c -- $(CFLAGS) 2>/dev/null || true

e2e: build
	@if [ -f tests/integration/blackbox.sh ]; then sh tests/integration/blackbox.sh; elif [ -f tests/smoke.sh ]; then sh tests/smoke.sh; fi

clean:
	rm -rf bin

