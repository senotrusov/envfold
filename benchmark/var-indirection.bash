#!/bin/bash

ITERATIONS=100000
VARNAMES=(TESTROOT)
__ENVFLD_L=("")

benchmark() {
    local TYPE=$1
    local start=$(date +%s%N)
    
    for ((i=0; i<ITERATIONS; i++)); do
        __ENVFLD_H=()
        __ENVFLD_O=()
        
        if [[ "$TYPE" == "hardcoded" ]]; then
            # --- HARDCODED LOGIC ---
            [[ -n "${TESTROOT+x}" ]] && { __ENVFLD_H[0]=1; __ENVFLD_O[0]="$TESTROOT"; } || __ENVFLD_H[0]=0
            if [[ "${TESTROOT:-}" == "${__ENVFLD_L[0]:-}" ]]; then
                if [[ ${__ENVFLD_H[0]:-0} -eq 1 ]]; then
                    export TESTROOT="${__ENVFLD_O[0]:-}"
                else
                    unset TESTROOT
                fi
            fi
            export TESTROOT='testroot-value'
        else
            # --- INDIRECT LOGIC ---
            for j in "${!VARNAMES[@]}"; do
                local vname="${VARNAMES[$j]}"
                [[ -n "${!vname+x}" ]] && { __ENVFLD_H[$j]=1; __ENVFLD_O[$j]="${!vname}"; } || __ENVFLD_H[$j]=0
                if [[ "${!vname:-}" == "${__ENVFLD_L[$j]:-}" ]]; then
                    if [[ ${__ENVFLD_H[$j]:-0} -eq 1 ]]; then
                        export "$vname"="${__ENVFLD_O[$j]:-}"
                    else
                        unset "$vname"
                    fi
                fi
                export "$vname"='testroot-value'
            done
        fi
    done
    
    local end=$(date +%s%N)
    local diff=$((end - start))
    local total_ms=$((diff / 1000000))
    local avg_ns=$((diff / ITERATIONS))
    
    printf "%-10s | Total: %7d ms | Avg: %5d ns/iter\n" "$TYPE" "$total_ms" "$avg_ns"
}

echo "Bash Benchmark ($ITERATIONS iterations)"
unset TESTROOT
benchmark "hardcoded"
unset TESTROOT
benchmark "indirect"
