#!/bin/sh
# Run each target separately: Go accepts only one fuzz target per invocation.
set -eu
cd "$(dirname "$0")/.."
export GOWORK=off CGO_ENABLED=0

duration=${NTHTML_FUZZ_DURATION:-10s}
workers=${NTHTML_FUZZ_WORKERS:-2}
for target in FuzzRender FuzzTTYNavigation FuzzSVGRewrite; do
    go test -mod=readonly ./document -run '^$' -fuzz "^${target}$" -fuzztime "$duration" -parallel "$workers"
done
go test -mod=readonly ./typeset -run '^$' -fuzz '^FuzzBuildWithSources$' -fuzztime "$duration" -parallel "$workers"
