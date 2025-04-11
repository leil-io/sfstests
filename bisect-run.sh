#!/bin/bash

# Script to run with bisect. Assuming current directory is saunafs, and
# sfstests source is under 'sfstests' directory
# ${1} should be test name.
# IMAGE_NAME Could optionally be Dockerfile path

: "${IMAGE_NAME:=sfstests/Dockerfile.test}"

if [ -z "$1" ]; then
	echo "Must supply test name or glob"
	exit 255
fi
cd sfstests || exit 255
go build -o sfstests
if [[ $? != 0 ]]; then
	echo "Could not build sfstests"
	exit 255
fi

cd ..
docker buildx build --build-arg BASE_IMAGE='ubuntu:24.04' --tag saunafs-test:latest -f "$IMAGE_NAME" .
if [[ $? != 0 ]]; then
	echo "Could not build image"
	exit 125
fi

for i in {1..5}; do
	echo "Try $i..."
	./sfstests/sfstests -w 1 -s SanityChecks,ShortSystemTests,SingleMachineTests,MachineTests,LongSystemTests -t "${1}"
	if [[ $? == 2 ]]; then
		docker rm $(docker stop $(docker ps -a -q --filter ancestor=saunafs-test --format="{{.ID}}")) || true
		docker image rm saunafs-test || true
		exit 1
	fi
done
docker rm $(docker stop $(docker ps -a -q --filter ancestor=saunafs-test --format="{{.ID}}")) || true
docker image rm saunafs-test || true
exit 0
