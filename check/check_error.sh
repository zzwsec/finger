#!/usr/bin/env bash

set -o nounset
set -o pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd) || exit 1
export ANSIBLE_CONFIG="${script_dir}/ansible.cfg"

inventory="${script_dir}/hosts"
date_arg="${1:-}"

if [[ -n "${date_arg}" ]]; then
    if [[ ! "${date_arg}" =~ ^[0-9]{8}$ ]]; then
        echo "用法: $0 [20060102]"
        exit 1
    fi

    log_date="${date_arg:0:4}-${date_arg:4:2}-${date_arg:6:2}"
    log_pattern="*${log_date}*.log"
else
    log_pattern="*.log"
fi

ansible servers \
    -i "${inventory}" \
    -m shell \
    -a "find /data -type f -name '${log_pattern}' -exec grep -Hn 'ERROR' {} + || true"