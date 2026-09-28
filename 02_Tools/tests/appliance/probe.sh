#!/bin/sh
# Copyright (c) 2026 DDP Engineering
# Licensed under the PolyForm Noncommercial License 1.0.0.
# See the repository LICENSE.txt for full license details.
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0

# Print the first version line while preserving the probed command's status.
probe() {
    probe_name=$1
    shift
    probe_output=$(mktemp) || return 1
    if "$@" >"$probe_output" 2>&1; then
        printf '%-16s ' "$probe_name"
        head -n 1 "$probe_output"
        rm -f "$probe_output"
        return 0
    else
        probe_status=$?
        printf '%-16s ' "$probe_name" >&2
        head -n 1 "$probe_output" >&2
        echo "MISSING" >&2
        rm -f "$probe_output"
        return "$probe_status"
    fi
}
