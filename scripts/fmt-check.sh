#!/bin/sh

set -eu

if [ -n "$(go fmt ./...)" ]; then
  echo "gofmt changed files; rerun checks after formatting"
  exit 1
fi
