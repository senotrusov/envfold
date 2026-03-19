// Copyright 2026 Stanislav Senotrusov
//
// This work is dual-licensed under the Apache License, Version 2.0
// and the MIT License. Refer to the LICENSE file in the top-level directory
// for the full license terms.
//
// SPDX-License-Identifier: Apache-2.0 OR MIT

package main

import (
	"fmt"
	"strings"
)

func generateFish(builder *strings.Builder, zones []Zone, allVars []string, report bool) {
	generateFishHeader(builder, allVars)
	generateSaveFunctionFish(builder)
	generateRestoreFunctionFish(builder, report)
	generateParentMapFish(builder, zones)
	generateApplyOneZoneFunctionFish(builder, zones, report)
	generateApplyStackFunctionFish(builder)
	generateHookFunctionFish(builder, zones)
}

func generateFishHeader(builder *strings.Builder, allVars []string) {
	builder.WriteString("if not set -q __ENVFLD_ZONE\n")
	builder.WriteString("  set -g __ENVFLD_ZONE \"NONE\"\n")
	builder.WriteString("end\n")
	builder.WriteString("set -g -a __ENVFLD_C\n\n")

	builder.WriteString("set -g __ENVFLD_VARS")
	for _, v := range allVars {
		builder.WriteString(fmt.Sprintf(" \"%s\"", v))
	}
	builder.WriteString("\n\n")
}

func generateSaveFunctionFish(builder *strings.Builder) {
	builder.WriteString(`function __envfold_save_outer
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

`)
}

func generateRestoreFunctionFish(builder *strings.Builder, report bool) {
	builder.WriteString(`function __envfold_restore_outer
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
`)
	if report {
		builder.WriteString(`        if set -q $v
          set -e $v
          echo "envfold: removed $v" >&2
        end
`)
	} else {
		builder.WriteString(`        set -e $v
`)
	}
	builder.WriteString(`      end
    end
  end
end

`)
}

func generateParentMapFish(builder *strings.Builder, zones []Zone) {
	builder.WriteString("function __envfold_get_parent\n")
	builder.WriteString("  switch \"$argv[1]\"\n")
	for _, z := range getSortedZonesByID(zones) {
		if z.ParentID != -1 {
			builder.WriteString(fmt.Sprintf("    case %s\n      echo \"%s\"\n", z.Name(), z.ParentName()))
		}
	}
	builder.WriteString("    case '*'\n      echo \"NONE\"\n")
	builder.WriteString("  end\n")
	builder.WriteString("end\n\n")
}

func generateApplyOneZoneFunctionFish(builder *strings.Builder, zones []Zone, report bool) {
	builder.WriteString("function __envfold_apply_one_zone\n")
	builder.WriteString("  set -l zone \"$argv[1]\"\n")
	builder.WriteString("  switch \"$zone\"\n")
	for _, z := range getSortedZonesByID(zones) {
		builder.WriteString(fmt.Sprintf("    case %s\n", z.Name()))
		for _, ev := range z.Vars {
			escapedVal := escapeSingleQuotesFish(ev.Value)
			var expr string
			if ev.IsDynamic {
				expr = fmt.Sprintf("(eval %s)", escapedVal)
			} else {
				expr = escapedVal
			}

			if ev.IsDynamic && ev.Cache {
				cIdx := ev.CacheIndex + 1
				builder.WriteString(fmt.Sprintf("      if not set -q __ENVFLD_C[%d]\n", cIdx))
				builder.WriteString(fmt.Sprintf("        set -g __ENVFLD_C[%d] %s\n", cIdx, expr))
				builder.WriteString("      end\n")
				expr = fmt.Sprintf("\"$__ENVFLD_C[%d]\"", cIdx)
			}

			if ev.Prepend {
				if ev.Name == "PATH" {
					builder.WriteString(fmt.Sprintf("      set -gx %s %s $%s\n", ev.Name, expr, ev.Name))
				} else {
					sep := ""
					if ev.IsPath {
						sep = ":"
					}
					builder.WriteString(fmt.Sprintf("      set -gx %s %s%s\"$%s\"\n", ev.Name, expr, sep, ev.Name))
				}
			} else {
				if ev.Name == "PATH" {
					builder.WriteString(fmt.Sprintf("      set -gx %s (string split \":\" %s)\n", ev.Name, expr))
				} else {
					builder.WriteString(fmt.Sprintf("      set -gx %s %s\n", ev.Name, expr))
				}
			}
			if report {
				builder.WriteString(fmt.Sprintf("      echo \"envfold: added %s\" >&2\n", ev.Name))
			}
		}
	}
	builder.WriteString("  end\n")
	builder.WriteString("end\n\n")
}

func generateApplyStackFunctionFish(builder *strings.Builder) {
	builder.WriteString(`function __envfold_apply_stack
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

`)
}

func generateHookFunctionFish(builder *strings.Builder, zones []Zone) {
	builder.WriteString("function __envfold_hook --on-event fish_prompt\n")
	builder.WriteString("  set -l target_zone \"NONE\"\n")
	builder.WriteString("  set -l current_pwd \"$PWD\"\n")
	builder.WriteString("  set current_pwd (string replace -r '/+$' '' -- \"$current_pwd\")/\n")
	builder.WriteString("  switch \"$current_pwd\"\n")
	for _, z := range zones {
		pattern := formatZonePatternFish(z.Path)
		builder.WriteString(fmt.Sprintf("    case %s\n      set target_zone \"%s\"\n", pattern, z.Name()))
	}
	builder.WriteString("  end\n\n")

	builder.WriteString(`  if test "$target_zone" != "$__ENVFLD_ZONE"
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
`)
}
