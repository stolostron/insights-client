#!/usr/bin/env bash

set -euxo pipefail


KEY="$SHARED_DIR/private.pem"
chmod 400 "$KEY"

IP="$(cat "$SHARED_DIR/public_ip")"
HOST="ec2-user@$IP"
OPT=(-q -o "UserKnownHostsFile=/dev/null" -o "StrictHostKeyChecking=no" -i "$KEY")


scp "${OPT[@]}" -r ../insights-client "$HOST:/tmp/insights-client"
ssh "${OPT[@]}" "$HOST" /tmp/insights-client/build/run-e2e-tests.sh $COMPONENT_IMAGE_REF