CC ?= gcc
CFLAGS ?= -O2 -Wall -Wextra -std=gnu11

all: minict

minict: minict.c
	$(CC) $(CFLAGS) -o $@ $<

clean:
	rm -f minict

.PHONY: all clean
