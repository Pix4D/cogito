#! /bin/bash

# Returns the git tag if any, without the optional "v" prefix, otherwise the branch name.
# Works both locally and on Travis.


# Return the git tag if any, without the optional "v" prefix, otherwise the branch name.
#
# On Travis, no matter the git clone depth, we always get a detached HEAD, so we
# cannot just ask git what is our branch.
# In addition, the variable TRAVIS_BRANCH changes meaning when the build is a PR:
# it becomes the name of the merge target branch (so it would become "master").
# This is why we are forced to all this machinery :-/
do_travis()
{
    if [ -n "$TRAVIS_PULL_REQUEST_BRANCH" ]; then
        echo "$TRAVIS_PULL_REQUEST_BRANCH"
    elif [ -n "$TRAVIS_TAG" ]; then
        normalize_git_tag "$TRAVIS_TAG"
    else
        echo "$TRAVIS_BRANCH"
    fi
}

# Return the git tag if any, without the optional "v" prefix, otherwise the branch name.
do_local()
{
    GIT_TAG=$(git tag --points-at HEAD)
    if [ -n "$GIT_TAG" ]; then
        normalize_git_tag "$GIT_TAG"
    else
        git symbolic-ref --short HEAD
    fi
}

normalize_git_tag()
{
    TAG=$1
    echo "$TAG" | sed s/^v//
}

#
# main
#
if [ -n "$TRAVIS" ]; then
    do_travis
else
    do_local
fi
