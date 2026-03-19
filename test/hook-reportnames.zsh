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
        if [[ -n "${(P)v+x}" ]]; then
          unset "$v"
          echo "envfold: removed $v" >&2
        fi
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
      echo "envfold: added ROOT_VAR" >&2
      ;;
    zone_1)
      export TESTROOT='testroot-value'
      echo "envfold: added TESTROOT" >&2
      export LOCALVAR='test'
      echo "envfold: added LOCALVAR" >&2
      export DATE_VAR=$(eval 'od -vAn -N4 -tx4 < /dev/urandom')
      echo "envfold: added DATE_VAR" >&2
      if [[ -z "${__ENVFLD_C[1]:-}" ]]; then
        __ENVFLD_C[1]=$(eval 'od -vAn -N4 -tx4 < /dev/urandom')
      fi
      export DATE_VAR_CACHED="${__ENVFLD_C[1]}"
      echo "envfold: added DATE_VAR_CACHED" >&2
      export QUOTED_VAR='val'\''withquote'
      echo "envfold: added QUOTED_VAR" >&2
      export SPACED_VAR='val  spaced'
      echo "envfold: added SPACED_VAR" >&2
      export TILDE_VAR='/home/user/foo'
      echo "envfold: added TILDE_VAR" >&2
      export TILDE_VAR_EXACT='/home/foo'
      echo "envfold: added TILDE_VAR_EXACT" >&2
      export TILDE_VAR_MID='a~/foo'
      echo "envfold: added TILDE_VAR_MID" >&2
      export TILDE_PATH_NOT_PATH=':/bin:~/foo'
      echo "envfold: added TILDE_PATH_NOT_PATH" >&2
      ;;
    zone_2)
      export PATH='/home/user/prefix-that-does-not-exist':"${PATH:-}"
      echo "envfold: added PATH" >&2
      export TESTROOT='now-with-prefix-'"${TESTROOT:-}"
      echo "envfold: added TESTROOT" >&2
      export LOCALVAR='test-foo'
      echo "envfold: added LOCALVAR" >&2
      ;;
    zone_3)
      export PATH='/home/user/bin:/usr/bin:/home/user/local/bin:/home/foo'
      echo "envfold: added PATH" >&2
      ;;
    zone_4)
      export LOCALVAR='test-foo-bar'
      echo "envfold: added LOCALVAR" >&2
      ;;
    zone_5)
      export MULTI_VAR='applied-to-both'
      echo "envfold: added MULTI_VAR" >&2
      ;;
    zone_6)
      export MULTI_VAR='applied-to-both'
      echo "envfold: added MULTI_VAR" >&2
      ;;
    zone_7)
      export WILDCARD_VAR='matched'
      echo "envfold: added WILDCARD_VAR" >&2
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
