// Copyright 2026 Stanislav Senotrusov
//
// This work is dual-licensed under the Apache License, Version 2.0
// and the MIT License. Refer to the LICENSE file in the top-level directory
// for the full license terms.
//
// SPDX-License-Identifier: Apache-2.0 OR MIT

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// EnvVar represents a parsed environment variable configuration.
type EnvVar struct {
	Name    string
	Value   string
	Prepend bool
	IsPath  bool
}

// Zone groups multiple paths that share the exact same variable definitions.
type Zone struct {
	Paths []string
	Vars  []EnvVar
}

// main coordinates the initialization, parsing, and bash output generation.
func main() {
	if len(os.Args) < 3 || os.Args[1] != "hook" || os.Args[2] != "bash" {
		fmt.Fprintln(os.Stderr, "Usage: envscope hook bash")
		os.Exit(1)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting home dir: %v\n", err)
		os.Exit(1)
	}

	configPath := filepath.Join(homeDir, ".config", "envscope", "main.conf")
	zones, allVars, err := parseConfig(configPath, homeDir)
	if err != nil {
		// If file doesn't exist, exit silently so it doesn't break shell startup
		if os.IsNotExist(err) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error parsing config: %v\n", err)
		os.Exit(1)
	}

	generateBash(zones, allVars)
}

// parseConfig reads the envscope configuration and constructs Zone definitions.
func parseConfig(configPath, homeDir string) ([]Zone, map[string]bool, error) {
	file, err := os.Open(configPath)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	var zones []Zone
	var currentPaths []string
	var currentVars []EnvVar
	allVars := make(map[string]bool)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n")
		if len(strings.TrimSpace(line)) == 0 {
			continue
		}

		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			parseVarLine(line, &currentVars, allVars)
		} else {
			if len(currentVars) > 0 {
				zones = append(zones, Zone{Paths: currentPaths, Vars: currentVars})
				currentPaths = []string{}
				currentVars = []EnvVar{}
			}
			currentPaths = append(currentPaths, expandPath(line, homeDir))
		}
	}

	if len(currentPaths) > 0 && len(currentVars) > 0 {
		zones = append(zones, Zone{Paths: currentPaths, Vars: currentVars})
	}

	return zones, allVars, scanner.Err()
}

// parseVarLine extracts a single variable's configurations including its prepend modifiers.
func parseVarLine(line string, currentVars *[]EnvVar, allVars map[string]bool) {
	line = strings.TrimSpace(line)
	prepend := false
	if strings.HasPrefix(line, "+") {
		prepend = true
		line = line[1:]
	}

	parts := strings.SplitN(line, "=", 2)
	if len(parts) == 2 {
		name := parts[0]
		val := parts[1]
		if strings.HasPrefix(val, "~/") {
			val = "$HOME/" + val[2:]
		}

		*currentVars = append(*currentVars, EnvVar{
			Name:    name,
			Value:   val,
			Prepend: prepend,
			IsPath:  name == "PATH",
		})
		allVars[name] = true
	}
}

// expandPath translates the tilde prefix into an absolute user home directory path.
func expandPath(path, homeDir string) string {
	if strings.HasPrefix(path, "~") {
		return filepath.Join(homeDir, path[1:])
	}
	return path
}

// generateBash drives the construction of the Bash shell hook script output.
func generateBash(zones []Zone, allVars map[string]bool) {
	var builder strings.Builder

	generateBashHeader(&builder)
	generateSaveRestoreFunctions(&builder, allVars)
	generateApplyFunction(&builder, zones, allVars)
	generateHookFunction(&builder, zones)

	fmt.Print(builder.String())
}

// generateBashHeader sets up initial runtime states resilient against `set -u`.
func generateBashHeader(builder *strings.Builder) {
	builder.WriteString("__ENVSCP_ZONE=${__ENVSCP_ZONE:-\"NONE\"}\n\n")
}

// generateSaveRestoreFunctions creates isolated Bash functions that store outer state
// and revert variables to that exact layout when exiting zones, avoiding `set -u` faults.
func generateSaveRestoreFunctions(builder *strings.Builder, allVars map[string]bool) {
	builder.WriteString("__envscope_save_outer() {\n")
	for v := range allVars {
		builder.WriteString(fmt.Sprintf(`  if [[ -n "${%s+x}" ]]; then
    __ENVSCP_OUTER_HAD_%s=1
    __ENVSCP_OUTER_%s="$%s"
  else
    __ENVSCP_OUTER_HAD_%s=0
  fi
`, v, v, v, v, v))
	}
	builder.WriteString("}\n\n")

	builder.WriteString("__envscope_restore_outer() {\n")
	for v := range allVars {
		builder.WriteString(fmt.Sprintf(`  if [[ "${%s:-}" == "${__ENVSCP_LAST_%s:-}" ]]; then
    if [[ ${__ENVSCP_OUTER_HAD_%s:-0} -eq 1 ]]; then
      export %s="${__ENVSCP_OUTER_%s:-}"
    else
      unset %s
    fi
  fi
`, v, v, v, v, v, v))
	}
	builder.WriteString("}\n\n")
}

// generateApplyFunction constructs the bash `case` structure used to project
// nested paths onto variable modifications safely supporting prepends.
func generateApplyFunction(builder *strings.Builder, zones []Zone, allVars map[string]bool) {
	builder.WriteString("__envscope_apply_zone() {\n")
	builder.WriteString("  local zone=\"$1\"\n")
	builder.WriteString("  case \"$zone\" in\n")
	for i, z := range zones {
		builder.WriteString(fmt.Sprintf("    zone_%d)\n", i))
		for _, ev := range z.Vars {
			// Using `${VAR:-}` handles unbound prepends accurately without `set -u` triggering.
			if ev.Prepend {
				if ev.IsPath {
					builder.WriteString(fmt.Sprintf("      export %s=\"%s:${%s:-}\"\n", ev.Name, ev.Value, ev.Name))
				} else {
					builder.WriteString(fmt.Sprintf("      export %s=\"%s${%s:-}\"\n", ev.Name, ev.Value, ev.Name))
				}
			} else {
				builder.WriteString(fmt.Sprintf("      export %s=\"%s\"\n", ev.Name, ev.Value))
			}
		}
		builder.WriteString("      ;; \n")
	}
	builder.WriteString("  esac\n\n")

	// Log precisely what envscope enacted to separate user changes from tool changes.
	for v := range allVars {
		builder.WriteString(fmt.Sprintf("  __ENVSCP_LAST_%s=\"${%s:-}\"\n", v, v))
	}
	builder.WriteString("}\n\n")
}

// generateHookFunction produces the runtime prompt trigger evaluation loop
// implementing longest-match nested path sorting priority.
func generateHookFunction(builder *strings.Builder, zones []Zone) {
	type PathMatch struct {
		Path   string
		ZoneID string
	}
	var matches []PathMatch
	for i, z := range zones {
		for _, p := range z.Paths {
			matches = append(matches, PathMatch{Path: p, ZoneID: fmt.Sprintf("zone_%d", i)})
		}
	}
	// Sort longest paths first to give deepest nested folders priority.
	sort.Slice(matches, func(i, j int) bool {
		return len(matches[i].Path) > len(matches[j].Path)
	})

	builder.WriteString("__envscope_hook() {\n")
	builder.WriteString("  local target_zone=\"NONE\"\n")
	builder.WriteString("  case \"$PWD\" in\n")
	for _, m := range matches {
		builder.WriteString(fmt.Sprintf("    \"%s\" | \"%s/\"* ) target_zone=\"%s\" ;;\n", m.Path, m.Path, m.ZoneID))
	}
	builder.WriteString("  esac\n\n")

	// Evaluates the current zone versus the known state, calling out to save/restore logic
	// seamlessly without needing a separate IN_ZONE tracker boolean.
	builder.WriteString(`  if [[ "$target_zone" != "NONE" ]]; then
    if [[ "${__ENVSCP_ZONE:-NONE}" == "NONE" ]]; then
      __envscope_save_outer
      __envscope_apply_zone "$target_zone"
    elif [[ "$target_zone" != "${__ENVSCP_ZONE:-NONE}" ]]; then
      __envscope_restore_outer
      __envscope_apply_zone "$target_zone"
    fi
    __ENVSCP_ZONE="$target_zone"
  elif [[ "${__ENVSCP_ZONE:-NONE}" != "NONE" ]]; then
    __envscope_restore_outer
    __ENVSCP_ZONE="NONE"
  fi
}

# Attach to PROMPT_COMMAND using '|| true' to bypass 'set -e' if declare fails.
if [[ ! "${PROMPT_COMMAND:-}" =~ __envscope_hook ]] && [[ "${PROMPT_COMMAND[*]:-}" != *__envscope_hook* ]]; then
  if [[ "$(declare -p PROMPT_COMMAND 2>/dev/null || true)" =~ "declare -a" ]]; then
    PROMPT_COMMAND+=("__envscope_hook")
  else
    PROMPT_COMMAND="${PROMPT_COMMAND:+${PROMPT_COMMAND}; }__envscope_hook"
  fi
fi
`)
}
