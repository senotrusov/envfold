typeset -g __ENVSCP_ZONE=${__ENVSCP_ZONE:-"NONE"}
typeset -g -a __ENVSCP_C 2>/dev/null || true

typeset -g -a __ENVSCP_VARS=(
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

__envscope_save_outer() {
  __ENVSCP_H=()
  __ENVSCP_O=()
  local i
  for i in {1..${#__ENVSCP_VARS[@]}}; do
    local v="${__ENVSCP_VARS[$i]}"
    if [[ -n "${(P)v+x}" ]]; then
      __ENVSCP_H[$i]=1
      __ENVSCP_O[$i]="${(P)v}"
    else
      __ENVSCP_H[$i]=0
    fi
  done
}

__envscope_restore_outer() {
  local i
  for i in {1..${#__ENVSCP_VARS[@]}}; do
    local v="${__ENVSCP_VARS[$i]}"
    if [[ "${(P)v:-}" == "${__ENVSCP_L[$i]:-}" ]]; then
      if [[ ${__ENVSCP_H[$i]:-0} -eq 1 ]]; then
        export "$v"="${__ENVSCP_O[$i]:-}"
      else
        if [[ -n "${(P)v+x}" ]]; then
          unset "$v"
          echo "envscope: removed $v" >&2
        fi
      fi
    fi
  done
}

typeset -g -A __ENVSCP_PARENT=(
  [zone_1]="zone_0"
  [zone_2]="zone_1"
  [zone_3]="zone_1"
  [zone_4]="zone_2"
  [zone_5]="zone_1"
  [zone_6]="zone_1"
  [zone_7]="zone_1"
)

__envscope_apply_one_zone() {
  local zone="$1"
  case "$zone" in
    zone_0)
      export ROOT_VAR='root-zone'
      echo "envscope: added ROOT_VAR" >&2
      ;;
    zone_1)
      export TESTROOT='testroot-value'
      echo "envscope: added TESTROOT" >&2
      export LOCALVAR='test'
      echo "envscope: added LOCALVAR" >&2
      export DATE_VAR=$(eval 'od -vAn -N4 -tx4 < /dev/urandom')
      echo "envscope: added DATE_VAR" >&2
      if [[ -z "${__ENVSCP_C[1]:-}" ]]; then
        __ENVSCP_C[1]=$(eval 'od -vAn -N4 -tx4 < /dev/urandom')
      fi
      export DATE_VAR_CACHED="${__ENVSCP_C[1]}"
      echo "envscope: added DATE_VAR_CACHED" >&2
      export QUOTED_VAR='val'\''withquote'
      echo "envscope: added QUOTED_VAR" >&2
      export SPACED_VAR='val  spaced'
      echo "envscope: added SPACED_VAR" >&2
      export TILDE_VAR='/home/user/foo'
      echo "envscope: added TILDE_VAR" >&2
      export TILDE_VAR_EXACT='/home/foo'
      echo "envscope: added TILDE_VAR_EXACT" >&2
      export TILDE_VAR_MID='a~/foo'
      echo "envscope: added TILDE_VAR_MID" >&2
      export TILDE_PATH_NOT_PATH=':/bin:~/foo'
      echo "envscope: added TILDE_PATH_NOT_PATH" >&2
      ;;
    zone_2)
      export PATH='/home/user/prefix-that-does-not-exist':"${PATH:-}"
      echo "envscope: added PATH" >&2
      export TESTROOT='now-with-prefix-'"${TESTROOT:-}"
      echo "envscope: added TESTROOT" >&2
      export LOCALVAR='test-foo'
      echo "envscope: added LOCALVAR" >&2
      ;;
    zone_3)
      export PATH='/home/user/bin:/usr/bin:/home/user/local/bin:/home/foo'
      echo "envscope: added PATH" >&2
      ;;
    zone_4)
      export LOCALVAR='test-foo-bar'
      echo "envscope: added LOCALVAR" >&2
      ;;
    zone_5)
      export MULTI_VAR='applied-to-both'
      echo "envscope: added MULTI_VAR" >&2
      ;;
    zone_6)
      export MULTI_VAR='applied-to-both'
      echo "envscope: added MULTI_VAR" >&2
      ;;
    zone_7)
      export WILDCARD_VAR='matched'
      echo "envscope: added WILDCARD_VAR" >&2
      ;;
  esac
}

__envscope_apply_stack() {
  local zone_id="$1"
  local stack=()
  while [[ -n "$zone_id" && "$zone_id" != "NONE" ]]; do
    stack=("$zone_id" "${stack[@]}")
    zone_id="${__ENVSCP_PARENT[$zone_id]:-NONE}"
  done
  local z
  for z in "${stack[@]}"; do
    __envscope_apply_one_zone "$z"
  done
}

__envscope_hook() {
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

  if [[ "$target_zone" != "${__ENVSCP_ZONE:-NONE}" ]]; then
    if [[ "${__ENVSCP_ZONE:-NONE}" != "NONE" ]]; then
      __envscope_restore_outer
    fi
    if [[ "$target_zone" != "NONE" ]]; then
      if [[ "${__ENVSCP_ZONE:-NONE}" == "NONE" ]]; then
        __envscope_save_outer
      fi
      __envscope_apply_stack "$target_zone"
      __ENVSCP_L=()
      local i
      for i in {1..${#__ENVSCP_VARS[@]}}; do
        local v="${__ENVSCP_VARS[$i]}"
        __ENVSCP_L[$i]="${(P)v:-}"
      done
    else
      unset __ENVSCP_L __ENVSCP_O __ENVSCP_H
    fi
    __ENVSCP_ZONE="$target_zone"
  fi
}

autoload -Uz add-zsh-hook
add-zsh-hook precmd __envscope_hook
