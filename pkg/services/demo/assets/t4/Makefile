CC ?= gcc
CFLAGS = -Wall -Wextra -Werror -pedantic -std=c17 -O2 -Isrc

.PHONY: build test clean

build:
	mkdir -p bin
	$(CC) $(CFLAGS) -o bin/t4 src/t4.c src/http.c

test:
	mkdir -p bin
	$(CC) $(CFLAGS) -Itests/unit -o bin/test_http src/http.c tests/unit/test_http.c
	./bin/test_http

clean:
	rm -rf bin
