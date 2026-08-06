package services

import (
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

	t.Run("handles .mk extension", func(t *testing.T) {
		input := "target:\n  echo hello"
		expected := "target:\n\techo hello"
		got := normalizeMakefileTabs("rules.mk", input)
		if got != expected {
			t.Errorf("expected .mk file to be normalized")
		}
	})
}
