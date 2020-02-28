#!/bin/bash

# Build deb or rpm packages for honeytail.
set -e

function usage() {
    echo "Usage: build-pkg.sh -m <arch> -v <version> -t <package_type>"
    exit 2
}

while getopts "v:t:m:" opt; do
    case "$opt" in
    v)
        version=$OPTARG
        ;;
    t)
        pkg_type=$OPTARG
        ;;
    m)
        arch=$OPTARG
        ;;
    esac
done

if [ -z "$pkg_type" ] || [ -z "$arch" ]; then
    usage
fi

if [ -z "$version" ]; then
    version=v0.0.0-dev
fi

fpm -s dir -n honeytail \
    -m "Honeycomb <team@honeycomb.io>" \
    -v $version \
    -t $pkg_type \
    -a $arch \
    --pre-install=./preinstall \
    $GOPATH/bin/honeytail-linux-${arch}=/usr/bin/honeytail \
    ./honeytail.upstart=/etc/init/honeytail.conf \
    ./honeytail.service=/lib/systemd/system/honeytail.service \
    ./honeytail.conf=/etc/honeytail/honeytail.conf
