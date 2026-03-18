package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// EnvVar represents a parsed environment variable configuration.
type EnvVar struct {
	Name       string
	Value      string
	Prepend    bool
	IsPath     bool
	IsDynamic  bool
	Cache      bool
	CacheIndex int
}

// Zone represents a single path and its variable definitions.
type Zone struct {
	Path     string
	ID       string
	ParentID string
	Vars     []EnvVar
}

// main coordinates the initialization, parsing, and shell output generation.
func main() {
	configFlag := flag.String("c", "", "path to the configuration file")
	reportFlag := flag.Bool("reportvars", false, "report variable changes to stderr")
	flag.Parse()

	args := flag.Args()
	if len(args) < 2 || args[0] != "hook" {
		fmt.Fprintln(os.Stderr, "envscope: Usage: envscope [-c config] [-reportvars] hook <bash|zsh|fish>")
		os.Exit(1)
	}
	shell := args[1]

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "envscope: error getting home dir: %v\n", err)
		os.Exit(1)
	}

	configPath := *configFlag
	if configPath == "" {
		configPath = filepath.Join(homeDir, ".config", "envscope", "main.conf")
	}

	zones, allVars, err := parseConfig(configPath, homeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "envscope: error parsing config: %v\n", err)
		os.Exit(1)
	}

	generateHook(shell, zones, allVars, *reportFlag)
}

// resolveZonePath resolves a path for a zone definition from the config file.
// Paths starting with "/" are treated as absolute. All other paths are
// considered relative to the user's home directory.
func resolveZonePath(path, homeDir string) string {
	if strings.HasPrefix(path, "/") {
		return path
	}
	return filepath.Join(homeDir, path)
}

// parseConfig reads the envscope configuration, constructs Zone definitions,
// and builds the parent-child hierarchy between them.
func parseConfig(configPath, homeDir string) ([]Zone, []string, error) {
	file, err := os.Open(configPath)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	var zones []Zone
	var currentPaths []string
	var currentVars []EnvVar
	var allVars []string
	seenVars := make(map[string]bool)

	lineNum := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if len(trimmed) == 0 || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if len(currentPaths) > 0 {
				if err := parseVarLine(trimmed, homeDir, &currentVars, &allVars, seenVars); err != nil {
					return nil, nil, fmt.Errorf("line %d: %w", lineNum, err)
				}
			} else {
				return nil, nil, fmt.Errorf("line %d: variable definition without a preceding zone path: %q", lineNum, trimmed)
			}
		} else {
			if len(currentPaths) > 0 && len(currentVars) > 0 {
				for _, p := range currentPaths {
					zones = append(zones, Zone{Path: p, Vars: currentVars})
				}
				currentPaths = nil
				currentVars = nil
			}
			currentPaths = append(currentPaths, resolveZonePath(trimmed, homeDir))
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}

	if len(currentPaths) > 0 && len(currentVars) > 0 {
		for _, p := range currentPaths {
			zones = append(zones, Zone{Path: p, Vars: currentVars})
		}
	}

	// Sort by path length to make parent-finding evaluate longest potentials first.
	sort.Slice(zones, func(i, j int) bool {
		return len(zones[i].Path) < len(zones[j].Path)
	})

	// Assign IDs.
	for i := range zones {
		zones[i].ID = fmt.Sprintf("zone_%d", i)
	}

	// Establish parent-child relationships.
	for i := range zones {
		bestParentIdx := -1
		for j := range zones {
			if i == j {
				continue
			}
			if isSubPath(zones[j].Path, zones[i].Path) {
				if bestParentIdx == -1 || len(zones[j].Path) > len(zones[bestParentIdx].Path) {
					bestParentIdx = j
				}
			}
		}
		if bestParentIdx != -1 {
			zones[i].ParentID = zones[bestParentIdx].ID
		}
	}

	return zones, allVars, nil
}

// isSubPath checks if the child path is logically nested under the parent path.
// It supports wildcard '*' characters in the parent path to allow for complex subsets.
func isSubPath(parent, child string) bool {
	if parent == "/" {
		return true
	}
	parentPath := parent
	if !strings.HasSuffix(parentPath, "/") {
		parentPath += "/"
	}
	childPath := child
	if !strings.HasSuffix(childPath, "/") {
		childPath += "/"
	}

	// A zone is not considered a parent of an identical zone.
	if parentPath == childPath {
		return false
	}

	parts := strings.Split(parentPath, "*")
	var rxParts []string
	for _, p := range parts {
		rxParts = append(rxParts, regexp.QuoteMeta(p))
	}
	rxStr := "^" + strings.Join(rxParts, ".*")
	matched, _ := regexp.MatchString(rxStr, childPath)
	return matched
}

// parseVarLine extracts a single variable's configurations, parsing names, plain text strings,
// and dynamic commands safely, including cache directives from comments.
func parseVarLine(line, homeDir string, currentVars *[]EnvVar, allVars *[]string, seenVars map[string]bool) error {
	origLine := line
	prepend := false
	if strings.HasPrefix(line, "+") {
		prepend = true
		line = line[1:]
	}

	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid variable definition (missing '='): %q", origLine)
	}

	name := strings.TrimSpace(parts[0])
	if !isValidVarName(name) {
		return fmt.Errorf("invalid variable name: %q", origLine)
	}

	valWithComment := parts[1]
	val := valWithComment
	cache := false

	if commentIndex := strings.Index(valWithComment, "#"); commentIndex > -1 {
		commentPart := strings.TrimSpace(valWithComment[commentIndex+1:])
		if commentPart == "cache" {
			cache = true
			val = valWithComment[:commentIndex]
		}
	}

	val = strings.TrimSpace(val)

	if strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"") {
		return fmt.Errorf("complex shell syntax in double quotes is not supported yet: %q", origLine)
	}

	var isDynamic bool
	var processedVal string

	if strings.HasPrefix(val, "$(") && strings.HasSuffix(val, ")") {
		isDynamic = true
		processedVal = val[2 : len(val)-1]
	} else {
		isDynamic = false
		processedVal = expandTilde(val, homeDir, name == "PATH")
	}

	*currentVars = append(*currentVars, EnvVar{
		Name:      name,
		Value:     processedVal,
		Prepend:   prepend,
		IsPath:    name == "PATH",
		IsDynamic: isDynamic,
		Cache:     cache,
	})

	if !seenVars[name] {
		seenVars[name] = true
		*allVars = append(*allVars, name)
	}

	return nil
}

// isValidVarName checks if a string is a valid POSIX/Bash environment variable name.
func isValidVarName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 && !isAlphaOrUnderscore(r) {
			return false
		}
		if i > 0 && !isAlphaNumOrUnderscore(r) {
			return false
		}
	}
	return true
}

func isAlphaOrUnderscore(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_'
}

func isAlphaNumOrUnderscore(r rune) bool {
	return isAlphaOrUnderscore(r) || (r >= '0' && r <= '9')
}

// expandTilde performs shell-like tilde expansion for a variable value.
// It expands a leading tilde (~) or tilde-slash (~/) to the user's home directory.
// If isPath is true, it also expands tildes that immediately follow a colon (:)
// to support PATH-style lists.
func expandTilde(val, homeDir string, isPath bool) string {
	expand := func(s string) string {
		if s == "~" {
			return homeDir
		}
		if strings.HasPrefix(s, "~/") {
			return homeDir + s[1:]
		}
		return s
	}

	if isPath {
		parts := strings.Split(val, ":")
		for i, p := range parts {
			parts[i] = expand(p)
		}
		return strings.Join(parts, ":")
	}

	return expand(val)
}

// getSortedZonesByID returns a new slice of zones sorted numerically by their IDs
// for deterministic readabilty in maps and static case evaluation points.
func getSortedZonesByID(zones []Zone) []Zone {
	sorted := make([]Zone, len(zones))
	copy(sorted, zones)
	sort.Slice(sorted, func(i, j int) bool {
		id1, _ := strconv.Atoi(strings.TrimPrefix(sorted[i].ID, "zone_"))
		id2, _ := strconv.Atoi(strings.TrimPrefix(sorted[j].ID, "zone_"))
		return id1 < id2
	})
	return sorted
}

// generateHook coordinates caching requirements and triggers shell-specific builders.
func generateHook(shell string, zones []Zone, allVars []string, report bool) {
	// Sort longest paths first to give deepest nested folders priority.
	sort.Slice(zones, func(i, j int) bool {
		return len(zones[i].Path) > len(zones[j].Path)
	})

	// Pre-calculate deterministic integer indices for all dynamic cached variables.
	cacheCounter := 0
	for i := range zones {
		for j := range zones[i].Vars {
			if zones[i].Vars[j].Cache {
				zones[i].Vars[j].CacheIndex = cacheCounter
				cacheCounter++
			}
		}
	}

	var builder strings.Builder

	switch shell {
	case "bash":
		generateBash(&builder, zones, allVars, report)
	case "zsh":
		generateZsh(&builder, zones, allVars, report)
	case "fish":
		generateFish(&builder, zones, allVars, report)
	default:
		fmt.Fprintf(os.Stderr, "envscope: unsupported shell %q\n", shell)
		os.Exit(1)
	}

	fmt.Print(builder.String())
}

// escapeSingleQuotes implements safe string enclosure for Bash/Zsh by replacing
// any single quotes with an escaped version and wrapping the result.
func escapeSingleQuotes(s string) string {
	escaped := strings.ReplaceAll(s, "'", "'\\''")
	return fmt.Sprintf("'%s'", escaped)
}

// escapeSingleQuotesFish safely escapes strings for Fish shell parsing.
func escapeSingleQuotesFish(s string) string {
	escaped := strings.ReplaceAll(s, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "'", "\\'")
	return fmt.Sprintf("'%s'", escaped)
}

// formatZonePattern converts a zone path into a safely quoted case pattern.
func formatZonePattern(path string) string {
	matchPath := path
	if !strings.HasSuffix(matchPath, "/") {
		matchPath += "/"
	}

	parts := strings.Split(matchPath, "*")
	var res strings.Builder
	for i, p := range parts {
		if i > 0 {
			res.WriteString("*")
		}
		if p != "" {
			res.WriteString(escapeSingleQuotes(p))
		}
	}
	res.WriteString("*")
	return res.String()
}

// formatZonePatternFish applies formatZonePattern logic specifically for Fish escaping.
func formatZonePatternFish(path string) string {
	matchPath := path
	if !strings.HasSuffix(matchPath, "/") {
		matchPath += "/"
	}
	
	// Unlike Bash, Fish's case builtin interprets wildcards accurately even when
	// enclosed inside single quotes. We must fully quote the pattern to prevent
	// the shell from attempting filesystem globbing before the case command executes.
	matchPath += "*"

	return escapeSingleQuotesFish(matchPath)
}

// -----------------------------------------------------------------------------
// BASH GENERATION
// -----------------------------------------------------------------------------

func generateBash(builder *strings.Builder, zones []Zone, allVars []string, report bool) {
	generateBashHeader(builder)
	generateVarsArrayBash(builder, allVars)
	generateSaveFunctionBash(builder)
	generateRestoreFunctionBash(builder, report)
	generateParentMapBash(builder, zones)
	generateApplyOneZoneFunctionBash(builder, zones, report)
	generateApplyStackFunctionBash(builder)
	generateHookFunctionBash(builder, zones)
}

func generateBashHeader(builder *strings.Builder) {
	builder.WriteString("__ENVSCP_ZONE=${__ENVSCP_ZONE:-\"NONE\"}\n")
	builder.WriteString("declare -a __ENVSCP_C 2>/dev/null || true\n\n")
}

func generateVarsArrayBash(builder *strings.Builder, allVars []string) {
	builder.WriteString("declare -a __ENVSCP_VARS=(\n")
	for _, v := range allVars {
		builder.WriteString(fmt.Sprintf("  \"%s\"\n", v))
	}
	builder.WriteString(")\n\n")
}

func generateSaveFunctionBash(builder *strings.Builder) {
	builder.WriteString(`__envscope_save_outer() {
  __ENVSCP_H=()
  __ENVSCP_O=()
  for i in "${!__ENVSCP_VARS[@]}"; do
    local v="${__ENVSCP_VARS[$i]}"
    if [[ -n "${!v+x}" ]]; then
      __ENVSCP_H[$i]=1
      __ENVSCP_O[$i]="${!v}"
    else
      __ENVSCP_H[$i]=0
    fi
  done
}

`)
}

func generateRestoreFunctionBash(builder *strings.Builder, report bool) {
	builder.WriteString(`__envscope_restore_outer() {
  for i in "${!__ENVSCP_VARS[@]}"; do
    local v="${__ENVSCP_VARS[$i]}"
    if [[ "${!v:-}" == "${__ENVSCP_L[$i]:-}" ]]; then
      if [[ ${__ENVSCP_H[$i]:-0} -eq 1 ]]; then
        export "$v"="${__ENVSCP_O[$i]:-}"
      else
`)
	if report {
		builder.WriteString(`        if [[ -n "${!v+x}" ]]; then
          unset "$v"
          echo "envscope: removed $v" >&2
        fi
`)
	} else {
		builder.WriteString(`        unset "$v"
`)
	}
	builder.WriteString(`      fi
    fi
  done
}

`)
}

func generateParentMapBash(builder *strings.Builder, zones []Zone) {
	builder.WriteString("declare -A __ENVSCP_PARENT=(\n")
	for _, z := range getSortedZonesByID(zones) {
		if z.ParentID != "" {
			builder.WriteString(fmt.Sprintf("  [%s]=\"%s\"\n", z.ID, z.ParentID))
		}
	}
	builder.WriteString(")\n\n")
}

func generateApplyOneZoneFunctionBash(builder *strings.Builder, zones []Zone, report bool) {
	builder.WriteString("__envscope_apply_one_zone() {\n")
	builder.WriteString("  local zone=\"$1\"\n")
	builder.WriteString("  case \"$zone\" in\n")
	for _, z := range getSortedZonesByID(zones) {
		builder.WriteString(fmt.Sprintf("    %s)\n", z.ID))
		for _, ev := range z.Vars {
			escapedVal := escapeSingleQuotes(ev.Value)
			var expr string
			if ev.IsDynamic {
				expr = fmt.Sprintf("$(eval %s)", escapedVal)
			} else {
				expr = escapedVal
			}

			if ev.IsDynamic && ev.Cache {
				builder.WriteString(fmt.Sprintf("      if [[ -z \"${__ENVSCP_C[%d]:-}\" ]]; then\n", ev.CacheIndex))
				builder.WriteString(fmt.Sprintf("        __ENVSCP_C[%d]=%s\n", ev.CacheIndex, expr))
				builder.WriteString("      fi\n")
				expr = fmt.Sprintf("\"${__ENVSCP_C[%d]}\"", ev.CacheIndex)
			}

			if ev.Prepend {
				sep := ""
				if ev.IsPath {
					sep = ":"
				}
				builder.WriteString(fmt.Sprintf("      export %s=%s%s\"${%s:-}\"\n", ev.Name, expr, sep, ev.Name))
			} else {
				builder.WriteString(fmt.Sprintf("      export %s=%s\n", ev.Name, expr))
			}
			if report {
				builder.WriteString(fmt.Sprintf("      echo \"envscope: added %s\" >&2\n", ev.Name))
			}
		}
		builder.WriteString("      ;;\n")
	}
	builder.WriteString("  esac\n")
	builder.WriteString("}\n\n")
}

func generateApplyStackFunctionBash(builder *strings.Builder) {
	builder.WriteString(`__envscope_apply_stack() {
  local zone_id="$1"
  local stack=()
  while [[ -n "$zone_id" && "$zone_id" != "NONE" ]]; do
    stack=("$zone_id" "${stack[@]}")
    zone_id="${__ENVSCP_PARENT[$zone_id]:-NONE}"
  done
  for z in "${stack[@]}"; do
    __envscope_apply_one_zone "$z"
  done
}

`)
}

func generateHookFunctionBash(builder *strings.Builder, zones []Zone) {
	builder.WriteString("__envscope_hook() {\n")
	builder.WriteString("  local target_zone=\"NONE\"\n")
	builder.WriteString("  local current_pwd=\"${PWD:-}\"\n")
	builder.WriteString("  current_pwd=\"${current_pwd%/}/\"\n")
	builder.WriteString("  case \"$current_pwd\" in\n")
	for _, z := range zones {
		pattern := formatZonePattern(z.Path)
		builder.WriteString(fmt.Sprintf("    %s ) target_zone=\"%s\" ;;\n", pattern, z.ID))
	}
	builder.WriteString("  esac\n\n")

	var lastVarTracker strings.Builder
	lastVarTracker.WriteString(`      __ENVSCP_L=()
      for i in "${!__ENVSCP_VARS[@]}"; do
        local v="${__ENVSCP_VARS[$i]}"
        __ENVSCP_L[$i]="${!v:-}"
      done`)

	builder.WriteString(fmt.Sprintf(`  if [[ "$target_zone" != "${__ENVSCP_ZONE:-NONE}" ]]; then
    if [[ "${__ENVSCP_ZONE:-NONE}" != "NONE" ]]; then
      __envscope_restore_outer
    fi
    if [[ "$target_zone" != "NONE" ]]; then
      if [[ "${__ENVSCP_ZONE:-NONE}" == "NONE" ]]; then
        __envscope_save_outer
      fi
      __envscope_apply_stack "$target_zone"
%s
    else
      unset __ENVSCP_L __ENVSCP_O __ENVSCP_H
    fi
    __ENVSCP_ZONE="$target_zone"
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
`, lastVarTracker.String()))
}

// -----------------------------------------------------------------------------
// ZSH GENERATION
// -----------------------------------------------------------------------------

func generateZsh(builder *strings.Builder, zones []Zone, allVars []string, report bool) {
	generateZshHeader(builder)
	generateVarsArrayZsh(builder, allVars)
	generateSaveFunctionZsh(builder)
	generateRestoreFunctionZsh(builder, report)
	generateParentMapZsh(builder, zones)
	generateApplyOneZoneFunctionZsh(builder, zones, report)
	generateApplyStackFunctionZsh(builder)
	generateHookFunctionZsh(builder, zones)
}

func generateZshHeader(builder *strings.Builder) {
	builder.WriteString("typeset -g __ENVSCP_ZONE=${__ENVSCP_ZONE:-\"NONE\"}\n")
	builder.WriteString("typeset -g -a __ENVSCP_C 2>/dev/null || true\n\n")
}

func generateVarsArrayZsh(builder *strings.Builder, allVars []string) {
	builder.WriteString("typeset -g -a __ENVSCP_VARS=(\n")
	for _, v := range allVars {
		builder.WriteString(fmt.Sprintf("  \"%s\"\n", v))
	}
	builder.WriteString(")\n\n")
}

func generateSaveFunctionZsh(builder *strings.Builder) {
	builder.WriteString(`__envscope_save_outer() {
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

`)
}

func generateRestoreFunctionZsh(builder *strings.Builder, report bool) {
	builder.WriteString(`__envscope_restore_outer() {
  local i
  for i in {1..${#__ENVSCP_VARS[@]}}; do
    local v="${__ENVSCP_VARS[$i]}"
    if [[ "${(P)v:-}" == "${__ENVSCP_L[$i]:-}" ]]; then
      if [[ ${__ENVSCP_H[$i]:-0} -eq 1 ]]; then
        export "$v"="${__ENVSCP_O[$i]:-}"
      else
`)
	if report {
		builder.WriteString(`        if [[ -n "${(P)v+x}" ]]; then
          unset "$v"
          echo "envscope: removed $v" >&2
        fi
`)
	} else {
		builder.WriteString(`        unset "$v"
`)
	}
	builder.WriteString(`      fi
    fi
  done
}

`)
}

func generateParentMapZsh(builder *strings.Builder, zones []Zone) {
	builder.WriteString("typeset -g -A __ENVSCP_PARENT=(\n")
	for _, z := range getSortedZonesByID(zones) {
		if z.ParentID != "" {
			builder.WriteString(fmt.Sprintf("  [%s]=\"%s\"\n", z.ID, z.ParentID))
		}
	}
	builder.WriteString(")\n\n")
}

func generateApplyOneZoneFunctionZsh(builder *strings.Builder, zones []Zone, report bool) {
	builder.WriteString("__envscope_apply_one_zone() {\n")
	builder.WriteString("  local zone=\"$1\"\n")
	builder.WriteString("  case \"$zone\" in\n")
	for _, z := range getSortedZonesByID(zones) {
		builder.WriteString(fmt.Sprintf("    %s)\n", z.ID))
		for _, ev := range z.Vars {
			escapedVal := escapeSingleQuotes(ev.Value)
			var expr string
			if ev.IsDynamic {
				expr = fmt.Sprintf("$(eval %s)", escapedVal)
			} else {
				expr = escapedVal
			}

			if ev.IsDynamic && ev.Cache {
				cIdx := ev.CacheIndex + 1
				builder.WriteString(fmt.Sprintf("      if [[ -z \"${__ENVSCP_C[%d]:-}\" ]]; then\n", cIdx))
				builder.WriteString(fmt.Sprintf("        __ENVSCP_C[%d]=%s\n", cIdx, expr))
				builder.WriteString("      fi\n")
				expr = fmt.Sprintf("\"${__ENVSCP_C[%d]}\"", cIdx)
			}

			if ev.Prepend {
				sep := ""
				if ev.IsPath {
					sep = ":"
				}
				builder.WriteString(fmt.Sprintf("      export %s=%s%s\"${%s:-}\"\n", ev.Name, expr, sep, ev.Name))
			} else {
				builder.WriteString(fmt.Sprintf("      export %s=%s\n", ev.Name, expr))
			}
			if report {
				builder.WriteString(fmt.Sprintf("      echo \"envscope: added %s\" >&2\n", ev.Name))
			}
		}
		builder.WriteString("      ;;\n")
	}
	builder.WriteString("  esac\n")
	builder.WriteString("}\n\n")
}

func generateApplyStackFunctionZsh(builder *strings.Builder) {
	builder.WriteString(`__envscope_apply_stack() {
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

`)
}

func generateHookFunctionZsh(builder *strings.Builder, zones []Zone) {
	builder.WriteString("__envscope_hook() {\n")
	builder.WriteString("  local target_zone=\"NONE\"\n")
	builder.WriteString("  local current_pwd=\"${PWD:-}\"\n")
	builder.WriteString("  current_pwd=\"${current_pwd%/}/\"\n")
	builder.WriteString("  case \"$current_pwd\" in\n")
	for _, z := range zones {
		pattern := formatZonePattern(z.Path)
		builder.WriteString(fmt.Sprintf("    %s ) target_zone=\"%s\" ;;\n", pattern, z.ID))
	}
	builder.WriteString("  esac\n\n")

	var lastVarTracker strings.Builder
	lastVarTracker.WriteString(`      __ENVSCP_L=()
      local i
      for i in {1..${#__ENVSCP_VARS[@]}}; do
        local v="${__ENVSCP_VARS[$i]}"
        __ENVSCP_L[$i]="${(P)v:-}"
      done`)

	builder.WriteString(fmt.Sprintf(`  if [[ "$target_zone" != "${__ENVSCP_ZONE:-NONE}" ]]; then
    if [[ "${__ENVSCP_ZONE:-NONE}" != "NONE" ]]; then
      __envscope_restore_outer
    fi
    if [[ "$target_zone" != "NONE" ]]; then
      if [[ "${__ENVSCP_ZONE:-NONE}" == "NONE" ]]; then
        __envscope_save_outer
      fi
      __envscope_apply_stack "$target_zone"
%s
    else
      unset __ENVSCP_L __ENVSCP_O __ENVSCP_H
    fi
    __ENVSCP_ZONE="$target_zone"
  fi
}

autoload -Uz add-zsh-hook
add-zsh-hook precmd __envscope_hook
`, lastVarTracker.String()))
}

// -----------------------------------------------------------------------------
// FISH GENERATION
// -----------------------------------------------------------------------------

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
	builder.WriteString("if not set -q __ENVSCP_ZONE\n")
	builder.WriteString("  set -g __ENVSCP_ZONE \"NONE\"\n")
	builder.WriteString("end\n")
	builder.WriteString("set -g -a __ENVSCP_C\n\n")

	builder.WriteString("set -g __ENVSCP_VARS")
	for _, v := range allVars {
		builder.WriteString(fmt.Sprintf(" \"%s\"", v))
	}
	builder.WriteString("\n\n")
}

func generateSaveFunctionFish(builder *strings.Builder) {
	builder.WriteString(`function __envscope_save_outer
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

`)
}

func generateRestoreFunctionFish(builder *strings.Builder, report bool) {
	builder.WriteString(`function __envscope_restore_outer
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
`)
	if report {
		builder.WriteString(`        if set -q $v
          set -e $v
          echo "envscope: removed $v" >&2
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
	builder.WriteString("function __envscope_get_parent\n")
	builder.WriteString("  switch \"$argv[1]\"\n")
	for _, z := range getSortedZonesByID(zones) {
		if z.ParentID != "" {
			builder.WriteString(fmt.Sprintf("    case %s\n      echo \"%s\"\n", z.ID, z.ParentID))
		}
	}
	builder.WriteString("    case '*'\n      echo \"NONE\"\n")
	builder.WriteString("  end\n")
	builder.WriteString("end\n\n")
}

func generateApplyOneZoneFunctionFish(builder *strings.Builder, zones []Zone, report bool) {
	builder.WriteString("function __envscope_apply_one_zone\n")
	builder.WriteString("  set -l zone \"$argv[1]\"\n")
	builder.WriteString("  switch \"$zone\"\n")
	for _, z := range getSortedZonesByID(zones) {
		builder.WriteString(fmt.Sprintf("    case %s\n", z.ID))
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
				builder.WriteString(fmt.Sprintf("      if not set -q __ENVSCP_C[%d]\n", cIdx))
				builder.WriteString(fmt.Sprintf("        set -g __ENVSCP_C[%d] %s\n", cIdx, expr))
				builder.WriteString("      end\n")
				expr = fmt.Sprintf("\"$__ENVSCP_C[%d]\"", cIdx)
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
				builder.WriteString(fmt.Sprintf("      echo \"envscope: added %s\" >&2\n", ev.Name))
			}
		}
	}
	builder.WriteString("  end\n")
	builder.WriteString("end\n\n")
}

func generateApplyStackFunctionFish(builder *strings.Builder) {
	builder.WriteString(`function __envscope_apply_stack
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

`)
}

func generateHookFunctionFish(builder *strings.Builder, zones []Zone) {
	builder.WriteString("function __envscope_hook --on-event fish_prompt\n")
	builder.WriteString("  set -l target_zone \"NONE\"\n")
	builder.WriteString("  set -l current_pwd \"$PWD\"\n")
	builder.WriteString("  set current_pwd (string replace -r '/+$' '' -- \"$current_pwd\")/\n")
	builder.WriteString("  switch \"$current_pwd\"\n")
	for _, z := range zones {
		pattern := formatZonePatternFish(z.Path)
		builder.WriteString(fmt.Sprintf("    case %s\n      set target_zone \"%s\"\n", pattern, z.ID))
	}
	builder.WriteString("  end\n\n")

	builder.WriteString(`  if test "$target_zone" != "$__ENVSCP_ZONE"
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
`)
}
