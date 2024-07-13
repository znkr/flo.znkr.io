#!/bin/bash

set -euo pipefail

TOKEN="<deSEC.io token>"
ZONE="dns.domain"
URL="https://desec.io/api/v1/domains/${ZONE}/rrsets"

# https://desec.readthedocs.io/en/latest/dns/rrsets.html#creating-an-rrset
add_record() {
    name=${1%%".${ZONE}"} # bash for "trim suffix"
    txtvalue=$2
    curl -s -X POST "${URL}/" \
        --header "Authorization: Token ${TOKEN}" \
        --header "Content-Type: application/json" --data @- <<EOB
{
    "subname": "${name}",
    "type": "TXT",
    "ttl": 3600,
    "records": ["\"${txtvalue}\""]
}
EOB
}

# https://desec.readthedocs.io/en/latest/dns/rrsets.html#creating-an-rrset
del_record() {
    name=${1%%".${ZONE}"}
    curl -s -X DELETE "${URL}/${name}/TXT/" \
        --header "Authorization: Token ${TOKEN}"
}

if [ "$#" -ne 4 ]; then
    echo "invalid number of parameters"
    exit 1
fi

case "${1}" in
    "set")
        add_record "${3}" "${4}"
        ;;
    "unset")
        del_record "${3}"
        ;;
    *)
        echo "unexpected commandline: ${@}"
        exit 1
        ;;
esac