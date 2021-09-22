#! /bin/bash
set -e

# If we are on a branch, we don't even look to see if there are associated tags.
BRANCH=$(git branch --show-current)
if [ -n "$BRANCH" ]; then
    echo "$BRANCH"
    exit 0
fi

#
# Here we are in detached HEAD state.
#

GIT_TAG=$(git tag --points-at HEAD)
if [ -z "$GIT_TAG" ]; then
    echo "$0: Error: detached HEAD but not git tag?" 1>&2
    exit 1
fi

if ! echo "$GIT_TAG" | grep -P '^v\d+\.\d+\.\d+$' > /dev/null; then
    # Tag is not a version, print it as-is.
    echo "$GIT_TAG"
    exit 0
fi

# Tag is indeed a version. Strip the prefix "v" and print it.
# (the `#` in the interpolation is equivalent to `^` in a regexp :-( )
echo "${GIT_TAG/#v/}"
exit 0

