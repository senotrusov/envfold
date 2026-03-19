#!/usr/bin/fish

set ITERATIONS 100000
set VARNAMES TESTROOT
set __ENVFLD_L ""

# Pre-calculate to avoid subshells in loops
set LOOP_RANGE (seq $ITERATIONS)
set VAR_INDICES (seq (count $VARNAMES))

function benchmark
    set -l TYPE $argv[1]
    set -l start (date +%s%N)
    
    for i in $LOOP_RANGE
        set -l __ENVFLD_H
        set -l __ENVFLD_O
        
        if test "$TYPE" = "hardcoded"
            if set -q TESTROOT
                set __ENVFLD_H[1] 1
                set __ENVFLD_O[1] "$TESTROOT"
            else
                set __ENVFLD_H[1] 0
            end

            if test "$TESTROOT" = "$__ENVFLD_L[1]"
                if test "$__ENVFLD_H[1]" -eq 1
                    set -gx TESTROOT "$__ENVFLD_O[1]"
                else
                    set -e TESTROOT
                end
            end
            set -gx TESTROOT 'testroot-value'
        else
            for j in $VAR_INDICES
                set -l vname $VARNAMES[$j]
                if set -q $vname
                    set __ENVFLD_H[$j] 1
                    set __ENVFLD_O[$j] "$$vname"
                else
                    set __ENVFLD_H[$j] 0
                end

                if test "$$vname" = "$__ENVFLD_L[$j]"
                    if test "$__ENVFLD_H[$j]" -eq 1
                        set -gx $vname "$__ENVFLD_O[$j]"
                    else
                        set -e $vname
                    end
                end
                set -gx $vname 'testroot-value'
            end
        end
    end

    set -l end (date +%s%N)
    set -l diff (math "$end - $start")
    set -l total_ms (math "$diff / 1000000")
    set -l avg_ns (math "$diff / $ITERATIONS")
    
    printf "%-10s | Total: %7.0f ms | Avg: %5.0f ns/iter\n" "$TYPE" "$total_ms" "$avg_ns"
end

echo "Fish Benchmark ($ITERATIONS iterations)"
set -e TESTROOT
benchmark "hardcoded"
set -e TESTROOT
benchmark "indirect"
