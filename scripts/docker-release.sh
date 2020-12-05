#!/usr/bin/env bash

image=edgerun/telemd

if [[ $1 ]]; then
	version="$1"
else
	version="latest"
fi

basetag="${image}:${version}"

# change into project root
BASE="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT=$(realpath "${BASE}/../")
cd $PROJECT_ROOT

# build all the images
docker build -t ${basetag}-amd64 -f build/package/telemd/Dockerfile.amd64 .
docker build -t ${basetag}-arm32v7 -f build/package/telemd/Dockerfile.arm32v7 .
docker build -t ${basetag}-aarch64 -f build/package/telemd/Dockerfile.aarch64 .

# # push em all
docker push ${basetag}-amd64 &
docker push ${basetag}-arm32v7 &
docker push ${basetag}-aarch64 &

wait

export DOCKER_CLI_EXPERIMENTAL=enabled

# create the manifest
docker manifest create ${basetag} \
	${basetag}-amd64 \
	${basetag}-arm32v7 \
	${basetag}-aarch64

# explicit annotations
docker manifest annotate ${basetag} ${basetag}-arm32v7 --os "linux" --arch "arm" --variant "v7"

# ship it
docker manifest push --purge ${basetag}
