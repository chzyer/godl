#!/bin/bash
version=$1
if [[ "$version" == "" ]]; then
	version=$(git tag | tail -n 1)
fi
if [[ "$version" != "" ]]; then
	git checkout $version
	version="-$version"
fi
export CGO_ENABLED=0
mkdir -p tmp/godl build

echo 'linux/amd64
darwin/amd64' | awk -F/ '{print "GOOS="$1" GOARCH="$2}' | while read line; do
	export $line
	go build -o tmp/godl/godl *.go
	cd tmp
	tar zcvf ../build/godl.$GOOS-${GOARCH}$version.tgz godl
	cd ../
done
rm -r tmp

if [[ "$version" != "" ]]; then
	git checkout -
fi
