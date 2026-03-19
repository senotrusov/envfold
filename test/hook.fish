if not set -q __ENVFLD_ZONE
  set -g __ENVFLD_ZONE "NONE"
end
set -g -a __ENVFLD_C

set -g __ENVFLD_VARS "TESTROOT" "LOCALVAR" "DATE_VAR" "DATE_VAR_CACHED" "QUOTED_VAR" "SPACED_VAR" "TILDE_VAR" "TILDE_VAR_EXACT" "TILDE_VAR_MID" "TILDE_PATH_NOT_PATH" "PATH" "MULTI_VAR" "WILDCARD_VAR" "ROOT_VAR"

function __envfold_save_outer
  set -g __ENVFLD_H
  set -g __ENVFLD_O
  if test (count $__ENVFLD_VARS) -eq 0
    return
  end
  for i in (seq 1 (count $__ENVFLD_VARS))
    set -l v $__ENVFLD_VARS[$i]
    if set -q $v
      set -a __ENVFLD_H 1
      set -a __ENVFLD_O (string join ":" $$v)
    else
      set -a __ENVFLD_H 0
      set -a __ENVFLD_O ""
    end
  end
end

function __envfold_restore_outer
  if test (count $__ENVFLD_VARS) -eq 0
    return
  end
  for i in (seq 1 (count $__ENVFLD_VARS))
    set -l v $__ENVFLD_VARS[$i]
    set -l current_val ""
    if set -q $v
      set current_val (string join ":" $$v)
    end
    set -l last_val ""
    if set -q __ENVFLD_L[$i]
      set last_val $__ENVFLD_L[$i]
    end

    if test "$current_val" = "$last_val"
      if test "$__ENVFLD_H[$i]" = "1"
        if test "$v" = "PATH"
          set -gx PATH (string split ":" "$__ENVFLD_O[$i]")
        else
          set -gx $v "$__ENVFLD_O[$i]"
        end
      else
        set -e $v
      end
    end
  end
end

function __envfold_get_parent
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

function __envfold_apply_one_zone
  set -l zone "$argv[1]"
  switch "$zone"
    case zone_0
      set -gx ROOT_VAR 'root-zone'
    case zone_1
      set -gx TESTROOT 'testroot-value'
      set -gx LOCALVAR 'test'
      set -gx DATE_VAR (eval 'od -vAn -N4 -tx4 < /dev/urandom')
      if not set -q __ENVFLD_C[1]
        set -g __ENVFLD_C[1] (eval 'od -vAn -N4 -tx4 < /dev/urandom')
      end
      set -gx DATE_VAR_CACHED "$__ENVFLD_C[1]"
      set -gx QUOTED_VAR 'val\'withquote'
      set -gx SPACED_VAR 'val  spaced'
      set -gx TILDE_VAR '/home/user/foo'
      set -gx TILDE_VAR_EXACT '/home/foo'
      set -gx TILDE_VAR_MID 'a~/foo'
      set -gx TILDE_PATH_NOT_PATH ':/bin:~/foo'
    case zone_2
      set -gx PATH '/home/user/prefix-that-does-not-exist' $PATH
      set -gx TESTROOT 'now-with-prefix-'"$TESTROOT"
      set -gx LOCALVAR 'test-foo'
    case zone_3
      set -gx PATH (string split ":" '/home/user/bin:/usr/bin:/home/user/local/bin:/home/foo')
    case zone_4
      set -gx LOCALVAR 'test-foo-bar'
    case zone_5
      set -gx MULTI_VAR 'applied-to-both'
    case zone_6
      set -gx MULTI_VAR 'applied-to-both'
    case zone_7
      set -gx WILDCARD_VAR 'matched'
  end
end

function __envfold_apply_stack
  set -l zone_id "$argv[1]"
  set -l stack
  while test -n "$zone_id" -a "$zone_id" != "NONE"
    set stack $zone_id $stack
    set zone_id (__envfold_get_parent "$zone_id")
  end
  for z in $stack
    __envfold_apply_one_zone "$z"
  end
end

function __envfold_hook --on-event fish_prompt
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

  if test "$target_zone" != "$__ENVFLD_ZONE"
    if test "$__ENVFLD_ZONE" != "NONE"
      __envfold_restore_outer
    end
    if test "$target_zone" != "NONE"
      if test "$__ENVFLD_ZONE" = "NONE"
        __envfold_save_outer
      end
      __envfold_apply_stack "$target_zone"
      set -g __ENVFLD_L
      if test (count $__ENVFLD_VARS) -gt 0
        for i in (seq 1 (count $__ENVFLD_VARS))
          set -l v $__ENVFLD_VARS[$i]
          if set -q $v
            set -a __ENVFLD_L (string join ":" $$v)
          else
            set -a __ENVFLD_L ""
          end
        end
      end
    else
      set -e __ENVFLD_L
      set -e __ENVFLD_O
      set -e __ENVFLD_H
    end
    set -g __ENVFLD_ZONE "$target_zone"
  end
end
