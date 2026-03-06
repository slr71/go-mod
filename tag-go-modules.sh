#!/usr/bin/env bash

# The purpose of this script is to add new tags for the next minor version of all of the Go modules. This is useful for
# things such as dependency updates, where most of the Go modules in this repository are being updated simultaneously.

set -e

function current_version {
    local module="$1"
    git tag | grep "^$module" | perl -pe 's{[^/]*/v}{}' | sort -t . -k 1 -k2 -k 3 -n | tail -n 1
}

# Make sure that we're in the expected directory.
script_dir=$(dirname "$0")
cd "$script_dir"

# Retag all of the go modules.
for module in *; do
    if [[ -d "$module" ]]; then
        current=$(current_version "$module")
        if [[ -n "$current" ]]; then
            next=$(echo "$current" | perl -pe 's/(\d+)$/$1 + 1/e')
            echo "$module/v$current => $module/v$next"
            git tag "$module/v$next"
        fi
    fi
done
