#!/bin/bash

commands=(
    "go run . https://pbs.twimg.com/media/EMtmPFLWkAA8CIS.jpg"
    "go run . https://golang.org/dl/go1.16.3.linux-amd64.tar.gz"
    "go run . http://ipv4.download.thinkbroadband.com/100MB.zip"
    "go run . -O=test_20MB.zip http://ipv4.download.thinkbroadband.com/20MB.zip"
    "go run . -O=test_20MB.zip -P=~/Downloads/ http://ipv4.download.thinkbroadband.com/20MB.zip"
    "go run . --rate-limit=300k http://ipv4.download.thinkbroadband.com/20MB.zip"
    "go run . --rate-limit=700k http://ipv4.download.thinkbroadband.com/20MB.zip"
    "go run . --rate-limit=2M http://ipv4.download.thinkbroadband.com/20MB.zip"
    "go run . -i=downloads.txt"
    "go run . -B http://ipv4.download.thinkbroadband.com/20MB.zip"
    "go run . --mirror --convert-links http://corndog.io/"
    "go run . --mirror https://oct82.com/"
    "go run . --mirror --reject=gif https://oct82.com/"
    "go run . --mirror https://trypap.com/"
    "go run . --mirror -X=/img https://trypap.com/"
)

for cmd in "${commands[@]}"; do
    echo "Ready to run: $cmd"
    read -n 1 -s -r -p "Press any key to continue..."
    echo
    eval $cmd
done
