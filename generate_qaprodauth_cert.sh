#!/bin/bash
SERVER=qaprodauth.cloud.redhat.com
openssl s_client -connect $SERVER:443 2>/dev/null </dev/null | sed -ne '/-BEGIN CERTIFICATE-/,/-END CERTIFICATE-/p'