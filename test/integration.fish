#!/usr/bin/env fish
set -gx HOME (pwd)/test
set -g FAILURES 0

echo "FISH: Running Error Handling Tests"

function assert_error
  set -l name $argv[1]
  set -l conf_file $argv[2]
  set -l expected_err $argv[3]
  
  set -l output (bin/envfold -c "$conf_file" hook fish 2>&1 >/dev/null)
  set -l code $status
  
  if test $code -eq 0
    echo "FAIL: $name - expected non-zero exit code"
    set FAILURES (math $FAILURES + 1)
  else if not string match -q "*$expected_err*" "$output"
    echo "FAIL: $name - expected error containing '$expected_err', got '$output'"
    set FAILURES (math $FAILURES + 1)
  else
    echo "PASS: $name"
  end
end

function assert_error_output_empty
  set -l name $argv[1]
  set -l conf_file $argv[2]
  
  set -l stdout_output (bin/envfold -c "$conf_file" hook fish 2>/dev/null)
  
  if test -n "$stdout_output"
    echo "FAIL: $name - expected empty stdout on error, got: $stdout_output"
    set FAILURES (math $FAILURES + 1)
  else
    echo "PASS: $name - stdout is empty on error"
  end
end

assert_error "Missing config errors" "test/does-not-exist.conf" "no such file or directory"
assert_error_output_empty "Missing config stdout blank" "test/does-not-exist.conf"
assert_error "Variable without zone" "test/no-zone.conf" "variable definition without a preceding zone path"
assert_error "Invalid variable definition (missing '=')" "test/bad-var.conf" "invalid variable definition (missing '=')"
assert_error_output_empty "Bad config stdout blank" "test/bad-var.conf"
assert_error "Invalid variable definition (empty name)" "test/bad-var2.conf" "invalid variable name"
assert_error "Invalid variable name" "test/bad-var-name.conf" "invalid variable name"
assert_error "Unsupported double quotes" "test/bad-var-quotes.conf" "complex shell syntax in double quotes is not supported yet"

function assert_eq
  set -l name $argv[1]
  set -l actual $argv[2]
  set -l expected $argv[3]
  if test "$actual" != "$expected"
    echo "FAIL: $name - expected '$expected', got '$actual'"
    set FAILURES (math $FAILURES + 1)
  else
    echo "PASS: $name"
  end
end

function assert_empty
  set -l name $argv[1]
  set -l actual $argv[2]
  if test -n "$actual"
    echo "FAIL: $name - expected empty, got '$actual'"
    set FAILURES (math $FAILURES + 1)
  else
    echo "PASS: $name"
  end
end

echo "FISH: Running Integration Tests"

mkdir -p "$HOME/other" \
         "$HOME/test/foo/bar" \
         "$HOME/test/tilde" \
         "$HOME/test/multi-1" \
         "$HOME/test/multi-2" \
         "$HOME/test/wildcard/foo/bar/deep" \
         "$HOME/test/wildcard/another/bar"

set -gx PATH "/usr/bin:/bin"
set ORIGINAL_PATH (string join ":" $PATH)

bin/envfold -c test/test.conf hook fish | source

cd "$HOME/other"
__envfold_hook
assert_empty "LOCALVAR outside" "$LOCALVAR"
assert_eq "ROOT_VAR active everywhere" "$ROOT_VAR" "root-zone"

cd "$HOME/test"
__envfold_hook
assert_eq "TESTROOT in zone_0" "$TESTROOT" "testroot-value"
assert_eq "LOCALVAR in zone_0" "$LOCALVAR" "test"
assert_eq "QUOTED_VAR in zone_0" "$QUOTED_VAR" "val'withquote"
assert_eq "SPACED_VAR in zone_0" "$SPACED_VAR" "val  spaced"

assert_eq "Tilde at start" "$TILDE_VAR" "$HOME/foo"
assert_eq "Exact tilde" "$TILDE_VAR_EXACT" "$HOME"
assert_eq "Tilde in middle (no expansion)" "$TILDE_VAR_MID" "a~/foo"
assert_eq "Tilde after colon in non-PATH (no expansion)" "$TILDE_PATH_NOT_PATH" ":/bin:~/foo"

if test -z "$DATE_VAR"
  echo "FAIL: DATE_VAR is empty"
  set FAILURES (math $FAILURES + 1)
else
  echo "PASS: DATE_VAR is set to dynamic value"
end
set FIRST_DATE_VAR "$DATE_VAR"

set FIRST_DATE_VAR_CACHED "$DATE_VAR_CACHED"
if test -z "$FIRST_DATE_VAR_CACHED"
  echo "FAIL: DATE_VAR_CACHED is empty"
  set FAILURES (math $FAILURES + 1)
else
  echo "PASS: DATE_VAR_CACHED initially set"
end

cd "$HOME/test/foo"
__envfold_hook
assert_eq "LOCALVAR in zone_1" "$LOCALVAR" "test-foo"
assert_eq "TESTROOT in zone_1" "$TESTROOT" "now-with-prefix-testroot-value"
assert_eq "PATH in zone_1" (string join ":" $PATH) "$HOME/prefix-that-does-not-exist:$ORIGINAL_PATH"

cd "$HOME/test/foo/bar"
__envfold_hook
assert_eq "LOCALVAR in zone_2" "$LOCALVAR" "test-foo-bar"

cd "$HOME/test/foo"
__envfold_hook
assert_eq "LOCALVAR in zone_1" "$LOCALVAR" "test-foo"
assert_eq "TESTROOT in zone_1" "$TESTROOT" "now-with-prefix-testroot-value"
assert_eq "PATH in zone_1" (string join ":" $PATH) "$HOME/prefix-that-does-not-exist:$ORIGINAL_PATH"

cd "$HOME/other"
__envfold_hook
assert_empty "LOCALVAR restored" "$LOCALVAR"
assert_empty "TESTROOT restored" "$TESTROOT"
assert_empty "QUOTED_VAR restored" "$QUOTED_VAR"
assert_empty "SPACED_VAR restored" "$SPACED_VAR"
assert_empty "TILDE_VAR restored" "$TILDE_VAR"
assert_empty "TILDE_VAR_EXACT restored" "$TILDE_VAR_EXACT"
assert_empty "TILDE_VAR_MID restored" "$TILDE_VAR_MID"
assert_empty "TILDE_PATH_NOT_PATH restored" "$TILDE_PATH_NOT_PATH"
assert_eq "ROOT_VAR still active" "$ROOT_VAR" "root-zone"
assert_eq "PATH restored" (string join ":" $PATH) "$ORIGINAL_PATH"

cd "$HOME/test"
__envfold_hook
if test "$DATE_VAR" = "$FIRST_DATE_VAR"
  echo "FAIL: DATE_VAR did not change (expected dynamic re-evaluation, got '$FIRST_DATE_VAR')"
  set FAILURES (math $FAILURES + 1)
else
  echo "PASS: DATE_VAR was re-evaluated dynamically"
end
assert_eq "DATE_VAR_CACHED remains cached" "$DATE_VAR_CACHED" "$FIRST_DATE_VAR_CACHED"

set -gx LOCALVAR "manual-override"
cd "$HOME/other"
__envfold_hook
assert_eq "LOCALVAR manual override protected" "$LOCALVAR" "manual-override"

cd "$HOME/test/tilde"
__envfold_hook
assert_eq "Tilde after colon in PATH" (string join ":" $PATH) "$HOME/bin:/usr/bin:$HOME/local/bin:$HOME"

cd "/"
__envfold_hook
assert_eq "ROOT_VAR set in /" "$ROOT_VAR" "root-zone"

cd "/etc"
__envfold_hook
assert_eq "ROOT_VAR remains set in /etc" "$ROOT_VAR" "root-zone"

cd "$HOME/test"
__envfold_hook
assert_eq "LOCALVAR in zone_0 after /" "$LOCALVAR" "test"
assert_eq "ROOT_VAR is set in zone_0" "$ROOT_VAR" "root-zone"

cd "$HOME/test/multi-1"
__envfold_hook
assert_eq "MULTI_VAR in multi-1" "$MULTI_VAR" "applied-to-both"

cd "$HOME/test/multi-2"
__envfold_hook
assert_eq "MULTI_VAR in multi-2" "$MULTI_VAR" "applied-to-both"

cd "$HOME/test/wildcard/foo/bar"
__envfold_hook
assert_eq "WILDCARD_VAR in wildcard/foo/bar" "$WILDCARD_VAR" "matched"

cd "$HOME/test/wildcard/another/bar"
__envfold_hook
assert_eq "WILDCARD_VAR in wildcard/another/bar" "$WILDCARD_VAR" "matched"

cd "$HOME/test/wildcard/foo/bar/deep"
__envfold_hook
assert_eq "WILDCARD_VAR inherited in deep" "$WILDCARD_VAR" "matched"

cd "$HOME/other"
__envfold_hook
assert_empty "MULTI_VAR restored" "$MULTI_VAR"
assert_empty "WILDCARD_VAR restored" "$WILDCARD_VAR"

if test $FAILURES -gt 0
  echo "[X] FISH: $FAILURES test(s) failed."
  exit 1
else
  echo "[+] FISH: All integration tests passed."
  exit 0
end
