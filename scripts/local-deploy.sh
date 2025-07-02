#!/bin/bash

set -eo pipefail

_basedir=$(cd $(dirname $0)/..; pwd -P)

_cluster_name=vpn-indexer
_namespace=vpn-test

if [[ ! $KUPO_URL || ! $OGMIOS_URL ]]; then
	echo "You must specify the KUPO_URL and OGMIOS_URL env vars for local testing"
	exit 1
fi

if ! which k3d &>/dev/null; then
	echo "Could not find required 'k3d' binary"
	exit 1
fi

# Create temp dir for chart values
_tmpdir=$(mktemp -d)
trap 'rm -rf ${_tmpdir}' EXIT

cd ${_basedir}

# Build image
(
set -x
docker build -t blinklabs-io/vpn-indexer .
)

# Create cluster
(
set -x
( k3d cluster get ${_cluster_name} &>/dev/null && (k3d kubeconfig merge ${_cluster_name} && kubectl config use-context k3d-${_cluster_name}) ) || k3d cluster create ${_cluster_name}
)

# Install minio
cat > ${_tmpdir}/minio-values.yaml <<EOF
service:
  ports:
    api: 80
provisioning:
  enabled: true
  users:
    - username: testuser
      password: testpass
      policies: [readwrite]
defaultBuckets: test-bucket
EOF
(
set -x
helm uninstall --wait -n ${_namespace} minio || true
helm install \
	--create-namespace \
	--wait \
	-n ${_namespace} \
	minio \
	oci://registry-1.docker.io/bitnamicharts/minio \
	--values ${_tmpdir}/minio-values.yaml
)

# Import image into k3d cluster
(
set -x
k3d image import --cluster ${_cluster_name} blinklabs-io/vpn-indexer
)

cat > ${_tmpdir}/vpn-indexer-values.yaml <<EOF
image:
  repository: blinklabs-io/vpn-indexer
  tag: latest
s3:
  clientBucket: test-bucket
  endpoint: http://minio.${_namespace}.svc
extraEnv:
  AWS_REGION: us-east-1
  AWS_ACCESS_KEY_ID: testuser
  AWS_SECRET_ACCESS_KEY: testpass
  # TODO: replace me with explicit param in helm chart
  TXBUILDER_KUPO_URL: ${KUPO_URL}
  TXBUILDER_OGMIOS_URL: ${OGMIOS_URL}
ca:
  # The CA cert/key are the same ones used in the CA unit tests
  cert: |
    -----BEGIN CERTIFICATE-----
    MIIClzCCAgCgAwIBAgIULdRPwP+Ue5oxNvgG6RjBFgEtovAwDQYJKoZIhvcNAQEL
    BQAwVzELMAkGA1UEBhMCQVUxEzARBgNVBAgMClNvbWUtU3RhdGUxITAfBgNVBAoM
    GEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDEQMA4GA1UEAwwHVGVzdCBDQTAeFw0y
    NTA2MDUxODU1MTBaFw0yODEwMjExODU1MTBaMFcxCzAJBgNVBAYTAkFVMRMwEQYD
    VQQIDApTb21lLVN0YXRlMSEwHwYDVQQKDBhJbnRlcm5ldCBXaWRnaXRzIFB0eSBM
    dGQxEDAOBgNVBAMMB1Rlc3QgQ0EwgZ8wDQYJKoZIhvcNAQEBBQADgY0AMIGJAoGB
    AM25vK3+qvIdsYsdRBhoVnQa5pfG8UCODD1nGcFBujtRyNCZUQdyu0pX20LhRIUm
    cTByGCOPsZxNr/kAK5mgXmOMWr/0dyyd9KHmeIFmdZCb8wGUI70XeTWIkXLYbffS
    ttwaVV+dClb27FI7Pjzm3ZUMAJ7XifVpj0diVd94l81FAgMBAAGjYDBeMB0GA1Ud
    DgQWBBRbpGrNjgwN/Jj8aLAoe+5AdtOapzAfBgNVHSMEGDAWgBRbpGrNjgwN/Jj8
    aLAoe+5AdtOapzAPBgNVHRMBAf8EBTADAQH/MAsGA1UdDwQEAwIBBjANBgkqhkiG
    9w0BAQsFAAOBgQAq+D287IeZ3R+s4beNyb0z9U4q+XmgZC2H0UtsoP+nDzvnq6EU
    X5K0OZf3nKDQPV886jBYuqpXcYdk86ylQbPQJbvSzqGTxg/WTey4BPN51ojdYEvt
    sQbsfCZK4tx5Q7FwfL9uk+tybKtEyrGKLr+JH07OwKhtQpYGoVtiD6U6nQ==
    -----END CERTIFICATE-----
  key: |
    -----BEGIN PRIVATE KEY-----
    MIICdwIBADANBgkqhkiG9w0BAQEFAASCAmEwggJdAgEAAoGBAM25vK3+qvIdsYsd
    RBhoVnQa5pfG8UCODD1nGcFBujtRyNCZUQdyu0pX20LhRIUmcTByGCOPsZxNr/kA
    K5mgXmOMWr/0dyyd9KHmeIFmdZCb8wGUI70XeTWIkXLYbffSttwaVV+dClb27FI7
    Pjzm3ZUMAJ7XifVpj0diVd94l81FAgMBAAECgYEArJlQO4qWUVuoQVbkcrXXEsIf
    BOfcMJT8n+eILCPA41PSb3CyEtWnXNApHQtyOWPvQv32Up+UG9bx9K635cQua0U8
    HVuJbm4GO6P+Q/I7cW8uIJPEdBKKbJwZ379F/APGBAP0RD5rJQ1Y65jP1Ii1yOsV
    +Y2ayN7q00sIjkctbAECQQDvuEERGy3uIJGP5/YFkAEGuvV/QPyXYIE7TteFhzYr
    nmU+U1qUEATBhJpGWn6AA1b4rz2PKbksap+5MfMDmGFhAkEA27J2b0P2FdOldy8u
    OI+Tx5RFuz7dcjXV59fWnbRO9d0q8MDWDckZ9oqT2yLHQ5sZ1HMkQVDlhPnPc1/s
    PBqiZQJAKjjCxReLbHCyEq2haHNnqt7NFJ/GnYby3BZT4YHiKaaZYHPf9Uoo/Ei1
    v4R62WM9M0nyRr/rjIYvIbhJfC2foQJBAL7xAUw81eEsfE/0uohACSFZda2CurYr
    ogiJJ6cS8dlv6oUqJCABG0aSNGUteeABKlbh56244HJNJ4bP5KJsR50CQEFT6XaA
    rQ0aNyVXoRZrTewWsowzPAasprQhv9qUQPy14+iO9Nttfumge+r4Z6/oqYn9Fem2
    xvIsZvJsUWLOo/c=
    -----END PRIVATE KEY-----
crl:
  configMapNamespace: vpn-test
  configMapName: test-crl
  configMapKey: crl.pem
  updateInterval: 1m
EOF

# Install helm chart
(
set -x
helm uninstall --wait -n ${_namespace} vpn-indexer || true
helm install --create-namespace --wait -n ${_namespace} vpn-indexer ../helm-charts/charts/vpn-indexer --values ${_tmpdir}/vpn-indexer-values.yaml
)
