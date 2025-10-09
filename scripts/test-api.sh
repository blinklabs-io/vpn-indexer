#!/bin/bash

set -eo pipefail

API_BASE=http://localhost:8080/api

SUBMIT_URL=${SUBMIT_URL:-}

USER1_ADDR=${USER1_ADDR:-}
USER1_SKEY_FILE=${USER1_SKEY_FILE:-}
USER2_ADDR=${USER2_ADDR:-}
USER2_SKEY_FILE=${USER2_SKEY_FILE:-}

_region='us east-1'
_price=5000000
_duration=259200000

txHexToJson() {
	local tx_hex=$1
	local out_file=$2

	echo '{ "type": "Unwitnessed Tx ConwayEra", "description": "Ledger Cddl Format", "cborHex": "'${tx_hex}'" }' > ${out_file}
}

txJsonToRaw() {
	local tx_file=$1
	local out_file=$2

	jq -r .cborHex ${tx_file} | xxd -r -p > ${out_file}
}

signTx() {
	local tx_file=$1
	local skey_file=$2
	local out_file=$3

	echo "*** Signing transaction"
	cardano-cli latest transaction sign --tx-file ${tx_file} --signing-key-file ${skey_file} --out-file ${out_file}
}

submitTx() {
	local raw_tx_file=$1

	echo "*** Submitting transaction"
	if ! curl -s --fail-with-body -H "Content-type: application/cbor" ${SUBMIT_URL} --data-binary @${raw_tx_file}; then
		echo
		echo "*** Failed to submit TX with CBOR:"
		echo
		xxd -c 0 -p ${raw_tx_file}
		exit 1
	fi
	echo
}

apiClientList() {
	local owner_addr=$1
	local client_id=$2

	curl -s ${API_BASE}/client/list -d '{"clientAddress":"'${owner_addr}'"}' |
		jq '.[] | select(.id == "'${client_id}'")'
}

apiWaitForClient() {
	local owner_addr=$1
	local client_id=$2

	echo "*** Waiting for client ${client_id} with owner address ${owner_addr}"
	while true; do
		if apiClientList ${owner_addr} ${client_id} | \
			grep -q ${client_id}; then
			break
		fi
		sleep 10
	done
}

apiWaitForClientExpiry() {
	local owner_addr=$1
	local client_id=$2
	local old_expiry=$3

	echo "*** Waiting for client ${client_id} with owner address ${owner_addr} with updated expiry"
	while true; do
		if ! apiClientList ${owner_addr} ${client_id} | jq -r .expiration | \
			grep -q "${old_expiry}"; then
			break
		fi
		sleep 10
	done
}

if [[ ! $SUBMIT_URL ]]; then
	echo "You must provide the SUBMIT_URL env var"
	exit 1
fi

if [[ ! $USER1_ADDR || ! $USER1_SKEY_FILE || ! $USER2_ADDR || ! $USER2_SKEY_FILE ]]; then
	echo "You must provide the USER1_ADDR, USER1_SKEY_FILE, USER2_ADDR, and USER2_SKEY_FILE env vars"
	exit 1
fi

_tmpdir=$(mktemp -d)

#trap 'rm -rf ${_tmpdir}' EXIT

# Signup for user1
echo "*** Generating TX for signup for user1"
(
set -x
curl -s --fail-with-body ${API_BASE}/tx/signup -d '{"clientAddress":"'${USER1_ADDR}'","region":"'"${_region}"'","price":'${_price}',"duration":'${_duration}'}' > ${_tmpdir}/user1_api_signup.json
)
_client_id=$(jq -r .clientId ${_tmpdir}/user1_api_signup.json)
txHexToJson $(jq -r .txCbor ${_tmpdir}/user1_api_signup.json) ${_tmpdir}/user1_tx_signup_unsigned.json
signTx ${_tmpdir}/user1_tx_signup_unsigned.json ${USER1_SKEY_FILE} ${_tmpdir}/user1_tx_signup_signed.json
txJsonToRaw ${_tmpdir}/user1_tx_signup_signed.json ${_tmpdir}/user1_tx_signup_signed.raw
submitTx ${_tmpdir}/user1_tx_signup_signed.raw
apiWaitForClient ${USER1_ADDR} ${_client_id}
apiClientList ${USER1_ADDR} ${_client_id} | jq .

# Transfer from user1 to user2
echo "*** Generating TX for transfer from user1 to user2"
(
set -x
curl -s --fail-with-body ${API_BASE}/tx/transfer -d '{"paymentAddress":"'${USER1_ADDR}'","ownerAddress":"'${USER2_ADDR}'","clientId":"'${_client_id}'"}' > ${_tmpdir}/user1_api_transfer_user2.json
)
txHexToJson $(jq -r .txCbor ${_tmpdir}/user1_api_transfer_user2.json) ${_tmpdir}/user1_tx_transfer_user2_unsigned.json
signTx ${_tmpdir}/user1_tx_transfer_user2_unsigned.json ${USER1_SKEY_FILE} ${_tmpdir}/user1_tx_transfer_user2_signed.json
txJsonToRaw ${_tmpdir}/user1_tx_transfer_user2_signed.json ${_tmpdir}/user1_tx_transfer_user2_signed.raw
submitTx ${_tmpdir}/user1_tx_transfer_user2_signed.raw
apiWaitForClient ${USER2_ADDR} ${_client_id}
apiClientList ${USER2_ADDR} ${_client_id} | jq .

# Renew for user2
echo "*** Generating TX for renew for user2"
(
set -x
curl -s --fail-with-body ${API_BASE}/tx/renew -d '{"paymentAddress":"'${USER2_ADDR}'","region":"'"${_region}"'","price":'${_price}',"duration":'${_duration}',"clientId":"'${_client_id}'"}' > ${_tmpdir}/user2_api_renew.json
)
_old_expiry=$(apiClientList ${USER2_ADDR} ${_client_id} | jq -r .expiration)
txHexToJson $(jq -r .txCbor ${_tmpdir}/user2_api_renew.json) ${_tmpdir}/user2_tx_renew_unsigned.json
signTx ${_tmpdir}/user2_tx_renew_unsigned.json ${USER2_SKEY_FILE} ${_tmpdir}/user2_tx_renew_signed.json
txJsonToRaw ${_tmpdir}/user2_tx_renew_signed.json ${_tmpdir}/user2_tx_renew_signed.raw
submitTx ${_tmpdir}/user2_tx_renew_signed.raw
apiWaitForClientExpiry ${USER2_ADDR} ${_client_id} "${_old_expiry}"
apiClientList ${USER2_ADDR} ${_client_id}

# Renew and transfer from user2 to user1
echo "*** Generating TX for renew and transfer from user2 to user1"
(
set -x
curl -s --fail-with-body ${API_BASE}/tx/renew -d '{"paymentAddress":"'${USER2_ADDR}'","ownerAddress":"'${USER1_ADDR}'","region":"'"${_region}"'","price":'${_price}',"duration":'${_duration}',"clientId":"'${_client_id}'"}' > ${_tmpdir}/user2_api_renew_transfer_user1.json 
)
txHexToJson $(jq -r .txCbor ${_tmpdir}/user2_api_renew_transfer_user1.json) ${_tmpdir}/user2_tx_renew_transfer_user1_unsigned.json
signTx ${_tmpdir}/user2_tx_renew_transfer_user1_unsigned.json ${USER2_SKEY_FILE} ${_tmpdir}/user2_tx_renew_transfer_user1_signed.json
txJsonToRaw ${_tmpdir}/user2_tx_renew_transfer_user1_signed.json ${_tmpdir}/user2_tx_renew_transfer_user1_signed.raw
submitTx ${_tmpdir}/user2_tx_renew_transfer_user1_signed.raw
apiWaitForClient ${USER1_ADDR} ${_client_id}
apiClientList ${USER1_ADDR} ${_client_id} | jq .

# Renew for user1 paid by user2
echo "*** Generating TX for renew for user1 paid by user2"
(
set -x
curl -s --fail-with-body ${API_BASE}/tx/renew -d '{"paymentAddress":"'${USER2_ADDR}'","region":"'"${_region}"'","price":'${_price}',"duration":'${_duration}',"clientId":"'${_client_id}'"}' > ${_tmpdir}/user2_api_renew_user1.json
)
_old_expiry=$(apiClientList ${USER1_ADDR} ${_client_id} | jq -r .expiration)
txHexToJson $(jq -r .txCbor ${_tmpdir}/user2_api_renew_user1.json) ${_tmpdir}/user2_tx_renew_user1_unsigned.json
signTx ${_tmpdir}/user2_tx_renew_user1_unsigned.json ${USER2_SKEY_FILE} ${_tmpdir}/user2_tx_renew_user1_signed.json
txJsonToRaw ${_tmpdir}/user2_tx_renew_user1_signed.json ${_tmpdir}/user2_tx_renew_user1_signed.raw
submitTx ${_tmpdir}/user2_tx_renew_user1_signed.raw
apiWaitForClientExpiry ${USER1_ADDR} ${_client_id} "${_old_expiry}"
apiClientList ${USER1_ADDR} ${_client_id} | jq .
