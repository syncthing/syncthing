// Marco Ivaldi <raptor@0xdeadbeef.info>

#include <stdio.h>
#include <string.h>
#include <stdlib.h>

#define BUFSIZE 256

void copy_string1(char *string)
{
	char buf[BUFSIZE];
	char *ptr;

	// ruleid: raptor-write-into-stack-buffer
	snprintf(buf, BUFSIZE, "%s", string);

	ptr = (char *)malloc(BUFSIZE);

	// ok: raptor-write-into-stack-buffer
	snprintf(ptr, BUFSIZE, "%s", string);
}

void copy_string2(char *string)
{
	char buf[BUFSIZE];

	// ruleid: raptor-write-into-stack-buffer
	strlcpy(buf, string, BUFSIZE);

	// ok: raptor-write-into-stack-buffer
	strlcpy(buf, "Hello, world!", BUFSIZE);
}

void copy_string3(char *string)
{
	char buf[BUFSIZE];

	// ruleid: raptor-write-into-stack-buffer
	memcpy(buf, string, BUFSIZE);
}

void bad(int limit) 
{
	char buf[BUFSIZE];

	for (int i = 0; i < limit; i++) {
		// should be catched, but too many false positives
		buf[i] = "A";
	}
}

int main() 
{
	printf("Hello, World!");
	return 0;
}
