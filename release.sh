#!/bin/sh

USER=fiveai
REPO=bgp-exporter

if [ "$1" == "" ]; then
  echo "Please provide a tag (e.g. 0.0.5)"
fi

which github-release >/dev/null 2>&1
if [ $? -ne 0 ]; then
  echo "You need github-release to run this (https://github.com/aktau/github-release)"
  exit 1
fi

set -e

TAG=$1

go build -o bgp_exporter main.go

github-release release --user $USER --repo $REPO --tag $TAG --name "BGP Exporter"
github-release upload  --user $USER --repo $REPO --tag $TAG --name bgp_exporter-$TAG.linux-amd64  --file ./bgp_exporter
