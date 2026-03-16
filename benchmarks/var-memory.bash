#!/bin/bash
# works for both bash and zsh

N=1000
VAL="abcdefghij1234567890"

run_variables() {
    for ((i=0; i<N; i++)); do
        v1="$VAL"; v2="$VAL"; v3="$VAL"; v4="$VAL"; v5="$VAL"; v6="$VAL"; v7="$VAL"; v8="$VAL"; v9="$VAL"; v10="$VAL"
        v11="$VAL"; v12="$VAL"; v13="$VAL"; v14="$VAL"; v15="$VAL"; v16="$VAL"; v17="$VAL"; v18="$VAL"; v19="$VAL"; v20="$VAL"
        v21="$VAL"; v22="$VAL"; v23="$VAL"; v24="$VAL"; v25="$VAL"; v26="$VAL"; v27="$VAL"; v28="$VAL"; v29="$VAL"; v30="$VAL"
        v31="$VAL"; v32="$VAL"; v33="$VAL"; v34="$VAL"; v35="$VAL"; v36="$VAL"; v37="$VAL"; v38="$VAL"; v39="$VAL"; v40="$VAL"
        t=$v1; t=$v40 # Small sample read
    done
}

run_array() {
    declare -a arr
    for ((i=0; i<N; i++)); do
        for ((j=0; j<40; j++)); do
            arr[$j]="$VAL"
        done
        for ((j=0; j<40; j++)); do
            tmp="${arr[$j]}"
        done
    done
}

export -f run_variables run_array
export VAL N

echo "--- Memory Footprint Benchmark ---"

echo "Testing Variables..."
/usr/bin/time -v bash -c "run_variables" 2>&1 | grep "Maximum resident set size"

echo "Testing Array..."
/usr/bin/time -v bash -c "run_array" 2>&1 | grep "Maximum resident set size"
