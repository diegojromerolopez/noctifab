package services

import (
	"strings"
	"testing"
)

func TestNormalizeMakefileTabs(t *testing.T) {
	t.Run("converts space-indented recipe lines into tab-indented lines for Makefile", func(t *testing.T) {
		input := `CC = gcc
CFLAGS = -Wall

all: main.o
    $(CC) $(CFLAGS) -o main main.o

main.o: main.c
  $(CC) $(CFLAGS) -c main.c

clean:
	rm -f *.o main
`
		expected := `CC = gcc
CFLAGS = -Wall

all: main.o
	$(CC) $(CFLAGS) -o main main.o

main.o: main.c
	$(CC) $(CFLAGS) -c main.c

clean:
	rm -f *.o main
`
		got := normalizeMakefileTabs("Makefile", input)
		if got != expected {
			t.Errorf("expected normalized Makefile tabs:\n%s\ngot:\n%s", expected, got)
		}
	})

	t.Run("ignores non-Makefile files", func(t *testing.T) {
		input := "  hello world\n    indented code"
		got := normalizeMakefileTabs("main.c", input)
		if got != input {
			t.Errorf("expected non-Makefile file content to be unchanged")
		}
	})

	t.Run("unindents targets mistakenly indented with spaces", func(t *testing.T) {
		input := `format:
    python3 -m compileall -q src tests

    e2e:
    docker compose up
`
		expected := `format:
	python3 -m compileall -q src tests

e2e:
	docker compose up
`
		got := normalizeMakefileTabs("Makefile", input)
		if got != expected {
			t.Errorf("expected space-indented e2e: target to be unindented to column 0:\n%s\ngot:\n%s", expected, got)
		}
	})
}

func TestStandardizeMakefile(t *testing.T) {
	t.Run("adds standard targets when missing", func(t *testing.T) {
		input := "test:\n  pytest\n"
		got := StandardizeMakefile(input)
		if !strings.Contains(got, ".PHONY:") {
			t.Errorf("expected .PHONY: in standardized Makefile")
		}
		if !strings.Contains(got, "all: build") {
			t.Errorf("expected all: build in standardized Makefile")
		}
		if !strings.Contains(got, "clean:") {
			t.Errorf("expected clean: in standardized Makefile")
		}
		if !strings.Contains(got, "run:") {
			t.Errorf("expected run: in standardized Makefile")
		}
	})

	t.Run("preserves existing build and test targets", func(t *testing.T) {
		input := ".PHONY: test\n\nbuild:\n\tpython3 setup.py build\n\ntest:\n\tpytest\n"
		got := StandardizeMakefile(input)
		if !strings.Contains(got, "all: build") {
			t.Errorf("expected all: build alias added")
		}
		if !strings.Contains(got, "python3 setup.py build") {
			t.Errorf("expected existing build target preserved")
		}
	})
}
