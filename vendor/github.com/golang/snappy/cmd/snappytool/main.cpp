/*
To build the snappytool binary:
g++ main.cpp /usr/lib/libsnappy.a -o snappytool
or, if you have built the C++ snappy library from source:
g++ main.cpp /path/to/your/snappy/.libs/libsnappy.a -o snappytool
after running "make" from your snappy checkout directory.
*/

#include <errno.h>
#include <stdio.h>
#include <string.h>
#include <unistd.h>

#include "snappy.h"

#define N 1000000

char dst[N];
char src[N];

int main(int argc, char** argv) {
  // Parse args.
  if (argc != 2) {
    fprintf(stderr, "exactly one of -d or -e must be given\n");
    return 1;
  }
  bool decode = strcmp(argv[1], "-d") == 0;
  bool encode = strcmp(argv[1], "-e") == 0;
  if (decode == encode) {
    fprintf(stderr, "exactly one of -d or -e must be given\n");
    return 1;
  }

  // Read all of stdin into src[:s].
  size_t s = 0;
  while (1) {
    if (s == N) {
      fprintf(stderr, "input too large\n");
      return 1;
    }
    ssize_t n = read(0, src+s, N-s);
    if (n == 0) {
      break;
    }
    if (n < 0) {
      fprintf(stderr, "read error: %s\n", strerror(errno));
      // TODO: handle EAGAIN, EINTR?
      return 1;
    }
    s += n;
  }

  // Encode or decode src[:s] to dst[:d], and write to stdout.
  size_t d = 0;
  if (encode) {
    if (N < snappy::MaxCompressedLength(s)) {
      fprintf(stderr, "input too large after encoding\n");
      return 1;
    }
    snappy::RawCompress(src, s, dst, &d);
  } else {
    if (!snappy::GetUncompressedLength(src, s, &d)) {
      fprintf(stderr, "could not get uncompressed length\n");
      return 1;
    }
    if (N < d) {
      fprintf(stderr, "input too large after decoding\n");
      return 1;
    }
    if (!snappy::RawUncompress(src, s, dst)) {
      fprintf(stderr, "input was not valid Snappy-compressed data\n");
      return 1;
    }
  }
  write(1, dst, d);
  return 0;
}
