#! /bin/bash

# Returns "latest" if the repository HEAD is also a git tag; otherwise an
# empty string.
# Works both locally and on Travis.

if [ -n "$TRAVIS_TAG" ]; then
    echo "latest"
elif [ -n "$(git tag --points-at HEAD)" ]; then
    echo "latest"
else
    echo -n ""
fi
