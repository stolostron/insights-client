# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project
#!/bin/bash

echo " > Running build.sh"
set -e

export DOCKER_IMAGE_AND_TAG=${1}

make docker/build
