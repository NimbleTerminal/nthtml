#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
CGO_ENABLED=0 GOOS=js GOARCH=wasm go build -o web/app.wasm ./examples/demo
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/wasm_exec.js

mkdir -p web/licenses
cp licenses/fonts/* web/licenses/
