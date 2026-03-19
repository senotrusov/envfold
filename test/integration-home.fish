#!/usr/bin/env fish

set -gx HOME (pwd)/test
set -g FAILURES 0

mkdir -p "$HOME/sub"

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

echo "FISH: Running Home / No-Root Integration Tests"

bin/envfold -c test/home.conf hook fish | source

cd /tmp
__envfold_hook
assert_empty "HOME_VAR outside" "$HOME_VAR"

cd "$HOME"
__envfold_hook
assert_eq "HOME_VAR in ~" "$HOME_VAR" "home-base"

cd "$HOME/sub"
__envfold_hook
assert_eq "HOME_VAR inherited in sub" "$HOME_VAR" "home-base"
assert_eq "SUB_VAR in sub" "$SUB_VAR" "sub-level"

cd /etc
__envfold_hook
assert_empty "HOME_VAR restored" "$HOME_VAR"
assert_empty "SUB_VAR restored" "$SUB_VAR"

if test $FAILURES -gt 0
  echo "[X] $FAILURES home test(s) failed."
  exit 1
else
  echo "[+] All home integration tests passed."
  exit 0
end
