#!/bin/zsh
zmodload zsh/datetime

ITERATIONS=100000
VARNAMES=(TESTROOT)
__ENVFLD_L=("")

benchmark() {
    local TYPE=$1
    local start=$EPOCHREALTIME
    
    # Pre-setup variables to avoid initialization overhead
    local i j vname
    
    for ((i=0; i<ITERATIONS; i++)); do
        __ENVFLD_H=()
        __ENVFLD_O=()
        
        if [[ "$TYPE" == "hardcoded" ]]; then
            # Logic: Check if set
            [[ -n "${TESTROOT+x}" ]] && { __ENVFLD_H[1]=1; __ENVFLD_O[1]="$TESTROOT"; } || __ENVFLD_H[1]=0
            # Comparison
            if [[ "${TESTROOT:-}" == "${__ENVFLD_L[1]:-}" ]]; then
                if [[ ${__ENVFLD_H[1]:-0} -eq 1 ]]; then
                    export TESTROOT="${__ENVFLD_O[1]:-}"
                else
                    unset TESTROOT
                fi
            fi
            export TESTROOT='testroot-value'
        else
            # Logic: Indirect
            for j in {1..${#VARNAMES}}; do
                vname="${VARNAMES[$j]}"
                # Zsh way to check if indirect var is set: ${(P)+vname}
                if (( ${(P)+vname} )); then
                    __ENVFLD_H[$j]=1
                    __ENVFLD_O[$j]="${(P)vname}"
                else
                    __ENVFLD_H[$j]=0
                fi
                
                if [[ "${(P)vname:-}" == "${__ENVFLD_L[$j]:-}" ]]; then
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
    
    local end=$EPOCHREALTIME
    # Math: duration in seconds
    local diff=$(( end - start ))
    local total_ms=$(( diff * 1000 ))
    local avg_ns=$(( (diff * 1000000000) / ITERATIONS ))
    
    printf "%-10s | Total: %8.2f ms | Avg: %6.0f ns/iter\n" "$TYPE" "$total_ms" "$avg_ns"
}

echo "Zsh Benchmark ($ITERATIONS iterations)"
unset TESTROOT
benchmark "hardcoded"
unset TESTROOT
benchmark "indirect"
