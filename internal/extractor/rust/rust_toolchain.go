// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package rust

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// Toolchain kinds, as reported in rust_toolchain_kind.
const (
	ToolchainChannel = "channel" // a release channel or toolchain name
	ToolchainPath    = "path"    // a directory of binaries (never run here)
	ToolchainNone    = "none"    // no usable selection
)

// Toolchain is the toolchain a project selects through a rustup
// toolchain file, read without running rustup.
type Toolchain struct {
	Kind       string
	Channel    string
	Components []string
	Targets    []string
	Profile    string
	// File is the toolchain file, relative to the project directory. It
	// is found in the project directory or a parent, so it consists of
	// ".." elements and a fixed file name, never repository content.
	File string
}

// toolchainFileNames lists the names rustup reads, in its order of
// precedence within one directory: the legacy file wins over the TOML
// one (verified against rustup 1.29).
var toolchainFileNames = []string{"rust-toolchain", "rust-toolchain.toml"}

// maxToolchainFileSize bounds the read; a real toolchain file is a few
// lines long.
const maxToolchainFileSize = 64 << 10

// toolchainNamePattern accepts channel, toolchain, component and target
// names ("1.85.0", "nightly-2026-01-01", "rust-src",
// "wasm32-unknown-unknown"). Values come from the repository, so
// anything else is dropped rather than passed on to callers.
var toolchainNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)

var toolchainProfiles = map[string]bool{"minimal": true, "default": true, "complete": true}

// distributableChannelPattern matches the channels rustup downloads
// ("stable", "1.85", "nightly-2026-01-01", "1.85.0-x86_64-...");
// other names are custom toolchains linked into rustup.
var distributableChannelPattern = regexp.MustCompile(`^(stable|beta|nightly|[0-9]+\.[0-9]+(\.[0-9]+)?)(-|$)`)

type toolchainFile struct {
	Toolchain struct {
		Channel    string   `toml:"channel"`
		Path       string   `toml:"path"`
		Components []string `toml:"components"`
		Targets    []string `toml:"targets"`
		Profile    string   `toml:"profile"`
	} `toml:"toolchain"`
}

// detectToolchain finds the toolchain file rustup would read for the
// project and parses it. It returns the selection and any warnings; the
// warnings name the file and the problem but never repeat its content.
func detectToolchain(projectPath string) (Toolchain, []string) {
	none := Toolchain{Kind: ToolchainNone}

	start, err := filepath.Abs(projectPath)
	if err != nil {
		return none, nil
	}
	// rustup searches from the real working directory, so walk the
	// resolved path and compare it with the resolved workspace.
	realStart, err := filepath.EvalSymlinks(start)
	if err != nil {
		return none, nil
	}
	boundary, escapes := searchBoundary(realStart)
	if escapes {
		return none, []string{"search skipped: the project directory resolves outside the workspace"}
	}
	start = realStart

	for dir := start; ; dir = filepath.Dir(dir) {
		var found []string
		for _, name := range toolchainFileNames {
			if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
				found = append(found, name)
			}
		}
		if len(found) > 0 {
			rel, err := filepath.Rel(start, filepath.Join(dir, found[0]))
			if err != nil {
				return none, nil
			}
			rel = filepath.ToSlash(rel)
			var warnings []string
			if len(found) > 1 {
				warnings = append(warnings, fmt.Sprintf(
					"%s and %s.toml both exist; using %s, as rustup does", rel, rel, rel))
			}
			toolchain, more := readToolchainFile(filepath.Join(dir, found[0]), found[0], boundary)
			toolchain.File = rel
			for _, w := range more {
				warnings = append(warnings, rel+": "+w)
			}
			return toolchain, warnings
		}
		if dir == boundary || filepath.Dir(dir) == dir {
			return none, nil
		}
	}
}

// searchBoundary is the last directory searched, as a real path. Under
// GitHub Actions the search stops at the workspace root, so files
// outside the checkout play no part; otherwise it runs to the
// filesystem root, as rustup's does. Both paths are compared resolved,
// so a symlinked spelling of either cannot lift the boundary. escapes
// reports a project whose real path lies outside the workspace, whether
// written inside it or not, or a workspace that cannot be resolved:
// either way no search could stay inside the checkout.
func searchBoundary(realStart string) (boundary string, escapes bool) {
	workspace := os.Getenv("GITHUB_WORKSPACE")
	if workspace == "" {
		return "", false
	}
	realWorkspace, err := filepath.Abs(workspace)
	if err == nil {
		realWorkspace, err = filepath.EvalSymlinks(realWorkspace)
	}
	if err != nil || !isWithin(realWorkspace, realStart) {
		return "", true
	}
	return realWorkspace, false
}

// isWithin reports whether path is root or lies below it, lexically.
func isWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// readToolchainFile reads one toolchain file. A symlink is followed only
// to a regular file inside the search boundary, so the outputs cannot
// carry the content of a file elsewhere on the runner.
func readToolchainFile(path, name, boundary string) (Toolchain, []string) {
	none := Toolchain{Kind: ToolchainNone}

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return none, []string{"cannot be resolved; ignoring it"}
	}
	if boundary != "" {
		if root, err := filepath.EvalSymlinks(boundary); err != nil || !isWithin(root, resolved) {
			return none, []string{"links outside the workspace; ignoring it"}
		}
	}

	file, err := os.Open(resolved)
	if err != nil {
		return none, []string{"cannot be read; ignoring it"}
	}
	defer func() { _ = file.Close() }()
	if info, err := file.Stat(); err != nil || !info.Mode().IsRegular() {
		return none, []string{"is not a regular file; ignoring it"}
	}
	content, err := io.ReadAll(io.LimitReader(file, maxToolchainFileSize+1))
	if err != nil {
		return none, []string{"cannot be read; ignoring it"}
	}
	if len(content) > maxToolchainFileSize {
		return none, []string{"exceeds 64 KiB; ignoring it"}
	}
	return parseToolchainFile(name, content)
}

// parseToolchainFile interprets toolchain file content the way rustup
// does: a legacy rust-toolchain file holding one line names a toolchain;
// any other content is TOML with a [toolchain] table. rustup counts the
// lines of the raw content, where only one final LF or CRLF ends the
// last line, so a blank line before or after the name makes the file
// TOML, which it then rejects (rustup 1.29.1).
func parseToolchainFile(name string, content []byte) (Toolchain, []string) {
	none := Toolchain{Kind: ToolchainNone}

	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return none, []string{"is empty; rustup rejects it"}
	}
	oneLine := !strings.Contains(strings.TrimSuffix(string(content), "\n"), "\n")
	if name == "rust-toolchain" && oneLine {
		return classifyLegacyName(trimmed)
	}

	var parsed toolchainFile
	meta, err := toml.Decode(trimmed, &parsed)
	if err != nil {
		return none, []string{"is not a valid toolchain file; rustup rejects it"}
	}
	section := parsed.Toolchain

	// rustup accepts a path toolchain only as a lone absolute path; with
	// any other key, or relative, it rejects the file (rustup 1.29).
	if meta.IsDefined("toolchain", "path") {
		switch {
		case meta.IsDefined("toolchain", "channel"):
			return none, []string{"names both a channel and a path; rustup rejects it"}
		case meta.IsDefined("toolchain", "components") || meta.IsDefined("toolchain", "targets") ||
			meta.IsDefined("toolchain", "profile"):
			return none, []string{"sets toolchain options beside a path; rustup rejects it"}
		case !filepath.IsAbs(section.Path):
			return none, []string{"names a relative path; rustup rejects it"}
		}
		return Toolchain{Kind: ToolchainPath}, nil
	}

	toolchain := none
	var warnings []string
	switch {
	case meta.IsDefined("toolchain", "channel"):
		if toolchain, warnings = classifyChannel(section.Channel); toolchain.Kind == ToolchainNone {
			return none, warnings
		}
	case !meta.IsDefined("toolchain", "components") && !meta.IsDefined("toolchain", "targets"):
		return none, []string{"selects no toolchain; rustup rejects it"}
	}

	// rustup rejects an unknown profile, except for a custom (linked)
	// toolchain, where it ignores install options.
	if section.Profile != "" && !toolchainProfiles[section.Profile] {
		if toolchain.Kind != ToolchainChannel || distributableChannelPattern.MatchString(toolchain.Channel) {
			return none, []string{"names an unknown profile; rustup rejects it"}
		}
		warnings = append(warnings, "ignoring an unknown profile for a custom toolchain")
	} else {
		toolchain.Profile = section.Profile
	}

	var dropped int
	toolchain.Components, dropped = validNames(section.Components)
	if dropped > 0 {
		warnings = append(warnings, fmt.Sprintf("ignoring %d invalid component name(s)", dropped))
	}
	toolchain.Targets, dropped = validNames(section.Targets)
	if dropped > 0 {
		warnings = append(warnings, fmt.Sprintf("ignoring %d invalid target name(s)", dropped))
	}
	return toolchain, warnings
}

// classifyLegacyName sorts the one line of a legacy rust-toolchain file.
// rustup runs the binaries of a toolchain named there by an absolute
// path; any other name with a path separator it rejects.
func classifyLegacyName(name string) (Toolchain, []string) {
	if filepath.IsAbs(name) {
		return Toolchain{Kind: ToolchainPath}, nil
	}
	return classifyChannel(name)
}

// classifyChannel accepts a channel name. rustup rejects a name holding
// a path separator, even an absolute one, in a TOML channel key, and the
// reserved name "none" in either file form (rustup 1.29.1).
func classifyChannel(name string) (Toolchain, []string) {
	switch {
	case strings.ContainsAny(name, `/\`):
		return Toolchain{Kind: ToolchainNone}, []string{"names a path as a toolchain; rustup rejects it"}
	case name == "none":
		return Toolchain{Kind: ToolchainNone}, []string{"names the reserved toolchain none; rustup rejects it"}
	case toolchainNamePattern.MatchString(name):
		return Toolchain{Kind: ToolchainChannel, Channel: name}, nil
	default:
		return Toolchain{Kind: ToolchainNone}, []string{"names an invalid toolchain; ignoring it"}
	}
}

// validNames keeps the names matching toolchainNamePattern and counts
// the rest.
func validNames(names []string) ([]string, int) {
	var valid []string
	for _, name := range names {
		if toolchainNamePattern.MatchString(name) {
			valid = append(valid, name)
		}
	}
	return valid, len(names) - len(valid)
}

// applyToolchain records the toolchain selection and reports any
// warnings as workflow annotations.
func applyToolchain(projectPath string, languageSpecific map[string]interface{}) {
	toolchain, warnings := detectToolchain(projectPath)
	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "::warning::Rust toolchain file %s\n", warning)
	}

	languageSpecific["toolchain_kind"] = toolchain.Kind
	if toolchain.File != "" {
		languageSpecific["toolchain_file"] = toolchain.File
	}
	if toolchain.Channel != "" {
		languageSpecific["toolchain_channel"] = toolchain.Channel
	}
	if len(toolchain.Components) > 0 {
		languageSpecific["toolchain_components"] = toolchain.Components
	}
	if len(toolchain.Targets) > 0 {
		languageSpecific["toolchain_targets"] = toolchain.Targets
	}
	if toolchain.Profile != "" {
		languageSpecific["toolchain_profile"] = toolchain.Profile
	}
}
