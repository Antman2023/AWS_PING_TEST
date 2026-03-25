#!/bin/sh
set -eu

BIN_NAME="AWS_PING_TEST"

mkdir -p release

GOOS=windows GOARCH=386 go build -trimpath -o "release/${BIN_NAME}_windows_x86.exe" .
GOOS=windows GOARCH=amd64 go build -trimpath -o "release/${BIN_NAME}_windows_x64.exe" .
GOOS=linux GOARCH=386 go build -trimpath -o "release/${BIN_NAME}_linux_x86" .
GOOS=linux GOARCH=amd64 go build -trimpath -o "release/${BIN_NAME}_linux_x64" .
GOOS=linux GOARCH=arm go build -trimpath -o "release/${BIN_NAME}_linux_arm" .
GOOS=linux GOARCH=arm64 go build -trimpath -o "release/${BIN_NAME}_linux_arm64" .
