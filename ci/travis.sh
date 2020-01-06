#! /bin/bash

# Colors:
# https://linuxhint.com/tput-printf-and-shell-expansions-how-to-create-awesome-outputs-with-bash-scripts/
# https://unix.stackexchange.com/questions/269077/tput-setaf-color-table-how-to-determine-color-codes

fgCyan=$(tput setaf 6)
fgRed=$(tput setaf 1)
fgGreen=$(tput setaf 2)
txReset=$(tput sgr0)

run() {
    echo
    echo "${fgCyan}RUN: $* ${txReset}"
    if ! "$@" ; then
        echo "${fgRed}FAIL: $* ${txReset}"
        exit 1
    fi
    echo "${fgGreen}SUCCESS: $* ${txReset}"
}

run task ci-init
run task test
run task docker-build
run task docker-smoke
run task docker-push
