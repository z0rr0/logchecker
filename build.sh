#!/bin/bash

# Copyright (c) 2015, Alexander Zaytsev. All rights reserved.
# Use of this source code is governed by a MIT-style
# license that can be found in the LICENSE file.
#
# Build script for gsa
# -v - verbose mode
# -f - force mode

program="LogChecker"
gobin="`which go`"
gitbin="`which git`"
repo="github.com/z0rr0/logchecker/main"

if [ -z "$GOPATH" ]; then
    echo "ERROR: set $GOPATH env"
    exit 1
fi
if [ ! -x "$gobin" ]; then
    echo "ERROR: can't find 'go' binary"
    exit 2
fi
if [ ! -x "$gitbin" ]; then
    echo "ERROR: can't find 'git' binary"
    exit 3
fi

cd ${GOPATH}/src/${repo}
gittag="`$gitbin tag | sort --version-sort | tail -1`"
gitver="`$gitbin log --oneline | head -1 `"
build="`date --utc +\"%F %T\"` UTC"
version="$gittag git:${gitver:0:7} $build"

options=""
while getopts ":fv" opt; do
    case $opt in
        f)
            options="$options -a"
            ;;
        v)
            options="$options -v"
            echo "$program version: $version"
            ;;
        \?)
            echo "Invalid option: -$OPTARG" >&2
            ;;
    esac
done

$gobin install $options -ldflags "-X main.Version \"$version\"" $repo
exit 0
