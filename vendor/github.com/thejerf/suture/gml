#!/bin/bash

# This command wraps up the gometalinter invocation in the pre-commit hook
# so it can be used by other things.

# If used in a way that the "lintclean" file is in the current working
# directory, the contents of the lintclean directory will be added to
# this invocation, allowing you to filter out specific failures.

# Rationale:
# --exclude="composite literal uses unkeyed field" \
#  jbowers: I disagree with community on this, and side with the Go
#    creators. Keyed fields are used when you expect new fields to be
#    unimportant to you, and you want to keep compiling, i.e., a new
#    option that, since you weren't using it before, probably want to
#    keep not using it. By contrast, unkeyed fields are appropriate
#    when you expect changes to the struct to really matter to you,
#    i.e., it is discovered that something MUST have a bool field added
#    or it turns out to be logically gibberish. You can't say that
#    one or the other must always be used... each has their place.
#
# -D gocyclo
#  jbowers: I consider cyclomatic complexity a bit of a crock.

if [ `which gometalinter` == "" ]; then
    echo You need to run the \"install_buildtools\" script.
    exit 1
fi

EXTRA_ARGS=

if [ -e lintclean ]; then
    EXTRA_ARGS=$(cat lintclean)
fi

gometalinter \
    --exclude="composite literal uses unkeyed field" \
    -j 4 \
    -D gocyclo \
    -D aligncheck \
    -D gofmt \
    -D goimports \
    -D gotype \
    -D structcheck \
    -D varcheck \
    $EXTRA_ARGS \
    $*
