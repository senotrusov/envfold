typeset -g __ENVFLD_ZONE=${__ENVFLD_ZONE:-"NONE"}
typeset -g -a __ENVFLD_C 2>/dev/null || true

typeset -g -a __ENVFLD_VARS=(
  "TESTROOT"
  "LOCALVAR"
  "DATE_VAR"
  "DATE_VAR_CACHED"
  "QUOTED_VAR"
  "SPACED_VAR"
  "TILDE_VAR"
  "TILDE_VAR_EXACT"
  "TILDE_VAR_MID"
  "TILDE_PATH_NOT_PATH"
  "PATH"
  "MULTI_VAR"
  "WILDCARD_VAR"
  "ROOT_VAR"
)

__envfold_save_outer() {
  __ENVFLD_H=()
  __ENVFLD_O=()
  local i
  for i in {1..${#__ENVFLD_VARS[@]}}; do
    local v="${__ENVFLD_VARS[$i]}"
    if [[ -n "${(P)v+x}" ]]; then
      __ENVFLD_H[$i]=1
      __ENVFLD_O[$i]="${(P)v}"
    else
      __ENVFLD_H[$i]=0
    fi
  done
}

__envfold_restore_outer() {
  local i
  for i in {1..${#__ENVFLD_VARS[@]}}; do
    local v="${__ENVFLD_VARS[$i]}"
    if [[ "${(P)v:-}" == "${__ENVFLD_L[$i]:-}" ]]; then
      if [[ ${__ENVFLD_H[$i]:-0} -eq 1 ]]; then
        export "$v"="${__ENVFLD_O[$i]:-}"
      else
        unset "$v"
      fi
    fi
  done
}

typeset -g -A __ENVFLD_PARENT=(
  [zone_1]="zone_0"
  [zone_2]="zone_1"
  [zone_3]="zone_1"
  [zone_4]="zone_2"
  [zone_5]="zone_1"
  [zone_6]="zone_1"
  [zone_7]="zone_1"
)

__envfold_apply_one_zone() {
  local zone="$1"
  case "$zone" in
    zone_0)
      export ROOT_VAR='root-zone'
      ;;
    zone_1)
      export TESTROOT='testroot-value'
      export LOCALVAR='test'
      export DATE_VAR=$(eval 'od -vAn -N4 -tx4 < /dev/urandom')
      if [[ -z "${__ENVFLD_C[1]:-}" ]]; then
        __ENVFLD_C[1]=$(eval 'od -vAn -N4 -tx4 < /dev/urandom')
      fi
      export DATE_VAR_CACHED="${__ENVFLD_C[1]}"
      export QUOTED_VAR='val'\''withquote'
      export SPACED_VAR='val  spaced'
      export TILDE_VAR='/home/user/foo'
      export TILDE_VAR_EXACT='/home/foo'
      export TILDE_VAR_MID='a~/foo'
      export TILDE_PATH_NOT_PATH=':/bin:~/foo'
      ;;
    zone_2)
      export PATH='/home/user/prefix-that-does-not-exist':"${PATH:-}"
      export TESTROOT='now-with-prefix-'"${TESTROOT:-}"
      export LOCALVAR='test-foo'
      ;;
    zone_3)
      export PATH='/home/user/bin:/usr/bin:/home/user/local/bin:/home/foo'
      ;;
    zone_4)
      export LOCALVAR='test-foo-bar'
      ;;
    zone_5)
      export MULTI_VAR='applied-to-both'
      ;;
    zone_6)
      export MULTI_VAR='applied-to-both'
      ;;
    zone_7)
      export WILDCARD_VAR='matched'
      ;;
  esac
}

__envfold_apply_stack() {
  local zone_id="$1"
  local stack=()
  while [[ -n "$zone_id" && "$zone_id" != "NONE" ]]; do
    stack=("$zone_id" "${stack[@]}")
    zone_id="${__ENVFLD_PARENT[$zone_id]:-NONE}"
  done
  local z
  for z in "${stack[@]}"; do
    __envfold_apply_one_zone "$z"
  done
}

__envfold_hook() {
  local target_zone="NONE"
  local current_pwd="${PWD:-}"
  current_pwd="${current_pwd%/}/"
  case "$current_pwd" in
    '/home/user/test/wildcard/'*'/bar/'* ) target_zone="zone_7" ;;
    '/home/user/test/foo/bar/'* ) target_zone="zone_4" ;;
    '/home/user/test/multi-1/'* ) target_zone="zone_5" ;;
    '/home/user/test/multi-2/'* ) target_zone="zone_6" ;;
    '/home/user/test/tilde/'* ) target_zone="zone_3" ;;
    '/home/user/test/foo/'* ) target_zone="zone_2" ;;
    '/home/user/test/'* ) target_zone="zone_1" ;;
    '/'* ) target_zone="zone_0" ;;
  esac

  if [[ "$target_zone" != "${__ENVFLD_ZONE:-NONE}" ]]; then
    if [[ "${__ENVFLD_ZONE:-NONE}" != "NONE" ]]; then
      __envfold_restore_outer
    fi
    if [[ "$target_zone" != "NONE" ]]; then
      if [[ "${__ENVFLD_ZONE:-NONE}" == "NONE" ]]; then
        __envfold_save_outer
      fi
      __envfold_apply_stack "$target_zone"
      __ENVFLD_L=()
      local i
      for i in {1..${#__ENVFLD_VARS[@]}}; do
        local v="${__ENVFLD_VARS[$i]}"
        __ENVFLD_L[$i]="${(P)v:-}"
      done
    else
      unset __ENVFLD_L __ENVFLD_O __ENVFLD_H
    fi
    __ENVFLD_ZONE="$target_zone"
  fi
}

autoload -Uz add-zsh-hook
add-zsh-hook precmd __envfold_hook
