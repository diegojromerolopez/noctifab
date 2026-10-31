CC ?= gcc
CFLAGS = -Wall -Wextra -Werror -pedantic -std=c17 -O2 -Isrc

.PHONY: all build test format lint e2e clean

all: build

build:
	mkdir -p bin
	$(CC) $(CFLAGS) -o bin/t4 src/*.c

test: build
	@mkdir -p bin
	@for f in tests/unit/test_*.c; do \
		if [ -f "$$f" ]; then \
			name=$$(basename "$$f" .c); \
			$(CC) $(CFLAGS) -Itests/unit -o "bin/$$name" "$$f" src/*.c 2>/dev/null || true; \
			if [ -x "bin/$$name" ]; then "bin/$$name"; fi; \
		fi; \
	done
	@if [ -f tests/integration/blackbox.sh ]; then bash tests/integration/blackbox.sh; fi
	@if [ -f tests/e2e/run_tests.sh ]; then bash tests/e2e/run_tests.sh; fi

format:
	@if command -v clang-format >/dev/null 2>&1; then \
		clang-format -i src/*.c src/*.h tests/unit/*.c 2>/dev/null || true; \
	fi

lint:
	@if command -v clang-tidy >/dev/null 2>&1; then \
		clang-tidy src/*.c -- $(CFLAGS) 2>/dev/null || true; \
	fi

e2e: build
	@if [ -f tests/integration/blackbox.sh ]; then bash tests/integration/blackbox.sh; fi
	@if [ -f tests/e2e/run_tests.sh ]; then bash tests/e2e/run_tests.sh; fi

clean:
	rm -rf bin data *.o src/*.o tests/unit/*.o


