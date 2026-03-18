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
