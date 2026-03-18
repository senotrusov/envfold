if not set -q __ENVSCP_ZONE
  set -g __ENVSCP_ZONE "NONE"
end
set -g -a __ENVSCP_C

set -g __ENVSCP_VARS "TESTROOT" "LOCALVAR" "DATE_VAR" "DATE_VAR_CACHED" "QUOTED_VAR" "SPACED_VAR" "TILDE_VAR" "TILDE_VAR_EXACT" "TILDE_VAR_MID" "TILDE_PATH_NOT_PATH" "PATH" "MULTI_VAR" "WILDCARD_VAR" "ROOT_VAR"

function __envscope_save_outer
  set -g __ENVSCP_H
  set -g __ENVSCP_O
  if test (count $__ENVSCP_VARS) -eq 0
    return
  end
  for i in (seq 1 (count $__ENVSCP_VARS))
    set -l v $__ENVSCP_VARS[$i]
    if set -q $v
      set -a __ENVSCP_H 1
      set -a __ENVSCP_O (string join ":" $$v)
    else
      set -a __ENVSCP_H 0
      set -a __ENVSCP_O ""
    end
  end
end

function __envscope_restore_outer
  if test (count $__ENVSCP_VARS) -eq 0
    return
  end
  for i in (seq 1 (count $__ENVSCP_VARS))
    set -l v $__ENVSCP_VARS[$i]
    set -l current_val ""
    if set -q $v
      set current_val (string join ":" $$v)
    end
    set -l last_val ""
    if set -q __ENVSCP_L[$i]
      set last_val $__ENVSCP_L[$i]
    end

    if test "$current_val" = "$last_val"
      if test "$__ENVSCP_H[$i]" = "1"
        if test "$v" = "PATH"
          set -gx PATH (string split ":" "$__ENVSCP_O[$i]")
        else
          set -gx $v "$__ENVSCP_O[$i]"
        end
      else
        if set -q $v
          set -e $v
          echo "envscope: removed $v" >&2
        end
      end
    end
  end
end

function __envscope_get_parent
  switch "$argv[1]"
    case zone_1
      echo "zone_0"
    case zone_2
      echo "zone_1"
    case zone_3
      echo "zone_1"
    case zone_4
      echo "zone_2"
    case zone_5
      echo "zone_1"
    case zone_6
      echo "zone_1"
    case zone_7
      echo "zone_1"
    case '*'
      echo "NONE"
  end
end

function __envscope_apply_one_zone
  set -l zone "$argv[1]"
  switch "$zone"
    case zone_0
      set -gx ROOT_VAR 'root-zone'
      echo "envscope: added ROOT_VAR" >&2
    case zone_1
      set -gx TESTROOT 'testroot-value'
      echo "envscope: added TESTROOT" >&2
      set -gx LOCALVAR 'test'
      echo "envscope: added LOCALVAR" >&2
      set -gx DATE_VAR (eval 'od -vAn -N4 -tx4 < /dev/urandom')
      echo "envscope: added DATE_VAR" >&2
      if not set -q __ENVSCP_C[1]
        set -g __ENVSCP_C[1] (eval 'od -vAn -N4 -tx4 < /dev/urandom')
      end
      set -gx DATE_VAR_CACHED "$__ENVSCP_C[1]"
      echo "envscope: added DATE_VAR_CACHED" >&2
      set -gx QUOTED_VAR 'val\'withquote'
      echo "envscope: added QUOTED_VAR" >&2
      set -gx SPACED_VAR 'val  spaced'
      echo "envscope: added SPACED_VAR" >&2
      set -gx TILDE_VAR '/home/user/foo'
      echo "envscope: added TILDE_VAR" >&2
      set -gx TILDE_VAR_EXACT '/home/foo'
      echo "envscope: added TILDE_VAR_EXACT" >&2
      set -gx TILDE_VAR_MID 'a~/foo'
      echo "envscope: added TILDE_VAR_MID" >&2
      set -gx TILDE_PATH_NOT_PATH ':/bin:~/foo'
      echo "envscope: added TILDE_PATH_NOT_PATH" >&2
    case zone_2
      set -gx PATH '/home/user/prefix-that-does-not-exist' $PATH
      echo "envscope: added PATH" >&2
      set -gx TESTROOT 'now-with-prefix-'"$TESTROOT"
      echo "envscope: added TESTROOT" >&2
      set -gx LOCALVAR 'test-foo'
      echo "envscope: added LOCALVAR" >&2
    case zone_3
      set -gx PATH (string split ":" '/home/user/bin:/usr/bin:/home/user/local/bin:/home/foo')
      echo "envscope: added PATH" >&2
    case zone_4
      set -gx LOCALVAR 'test-foo-bar'
      echo "envscope: added LOCALVAR" >&2
    case zone_5
      set -gx MULTI_VAR 'applied-to-both'
      echo "envscope: added MULTI_VAR" >&2
    case zone_6
      set -gx MULTI_VAR 'applied-to-both'
      echo "envscope: added MULTI_VAR" >&2
    case zone_7
      set -gx WILDCARD_VAR 'matched'
      echo "envscope: added WILDCARD_VAR" >&2
  end
end

function __envscope_apply_stack
  set -l zone_id "$argv[1]"
  set -l stack
  while test -n "$zone_id" -a "$zone_id" != "NONE"
    set stack $zone_id $stack
    set zone_id (__envscope_get_parent "$zone_id")
  end
  for z in $stack
    __envscope_apply_one_zone "$z"
  end
end

function __envscope_hook --on-event fish_prompt
  set -l target_zone "NONE"
  set -l current_pwd "$PWD"
  set current_pwd (string replace -r '/+$' '' -- "$current_pwd")/
  switch "$current_pwd"
    case '/home/user/test/wildcard/*/bar/*'
      set target_zone "zone_7"
    case '/home/user/test/foo/bar/*'
      set target_zone "zone_4"
    case '/home/user/test/multi-1/*'
      set target_zone "zone_5"
    case '/home/user/test/multi-2/*'
      set target_zone "zone_6"
    case '/home/user/test/tilde/*'
      set target_zone "zone_3"
    case '/home/user/test/foo/*'
      set target_zone "zone_2"
    case '/home/user/test/*'
      set target_zone "zone_1"
    case '/*'
      set target_zone "zone_0"
  end

  if test "$target_zone" != "$__ENVSCP_ZONE"
    if test "$__ENVSCP_ZONE" != "NONE"
      __envscope_restore_outer
    end
    if test "$target_zone" != "NONE"
      if test "$__ENVSCP_ZONE" = "NONE"
        __envscope_save_outer
      end
      __envscope_apply_stack "$target_zone"
      set -g __ENVSCP_L
      if test (count $__ENVSCP_VARS) -gt 0
        for i in (seq 1 (count $__ENVSCP_VARS))
          set -l v $__ENVSCP_VARS[$i]
          if set -q $v
            set -a __ENVSCP_L (string join ":" $$v)
          else
            set -a __ENVSCP_L ""
          end
        end
      end
    else
      set -e __ENVSCP_L
      set -e __ENVSCP_O
      set -e __ENVSCP_H
    end
    set -g __ENVSCP_ZONE "$target_zone"
  end
end
