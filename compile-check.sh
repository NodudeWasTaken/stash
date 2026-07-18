#!/bin/sh
# backend compile check used during the upstream sync; run inside golang:1.25-alpine
set -e
apk add --no-cache git gcc musl-dev sqlite-dev >/dev/null 2>&1
mkdir -p ui/v2.5/build
touch ui/v2.5/build/index.html

# dataloaden emits a duplicate "time" import
for f in internal/api/loaders/*_gen.go; do
  awk '/^\t"time"$/{if(++n>1)next}1' "$f" > /tmp/fixed && mv /tmp/fixed "$f"
done

go build ./... && echo BUILD_OK
go vet ./pkg/postgres/ ./pkg/database/ ./pkg/sqlite/ && echo VET_OK
