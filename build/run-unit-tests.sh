#!/bin/bash

echo " > Running run-unit-tests.sh"
set -e
export DOCKER_IMAGE_AND_TAG=${1}

make deps
make lint
make test
make coverage

exit 0