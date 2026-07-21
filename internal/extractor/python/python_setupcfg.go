// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package python

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// SetupCfg represents the structure of a setup.cfg file
type SetupCfg struct {
	Metadata struct {
		Name            string   `ini:"name"`
		Version         string   `ini:"version"`
		Author          string   `ini:"author"`
		AuthorEmail     string   `ini:"author_email"`
		Maintainer      string   `ini:"maintainer"`
		MaintainerEmail string   `ini:"maintainer_email"`
		Description     string   `ini:"description"`
		LongDescription string   `ini:"long_description"`
		License         string   `ini:"license"`
		Keywords        string   `ini:"keywords"`
		Classifiers     []string `ini:"classifiers"`
		URL             string   `ini:"url"`
		ProjectURLs     string   `ini:"project_urls"`
		PythonRequires  string   `ini:"python_requires"`
	}
	Options struct {
		Packages           []string `ini:"packages"`
		InstallRequires    []string `ini:"install_requires"`
		PythonRequires     string   `ini:"python_requires"`
		IncludePackageData bool     `ini:"include_package_data"`
		ZipSafe            bool     `ini:"zip_safe"`
	}
}

// extractFromSetupCfg extracts metadata from setup.cfg using a
// continuation-aware INI parser. It handles classic declarative
// setuptools layouts, PBR-style configurations, and the older
// hyphen-separated key forms that pre-date PEP 8 alignment.
func (e *Extractor) extractFromSetupCfg(path string, metadata *extractor.ProjectMetadata) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read setup.cfg: %w", err)
	}

	cfg := parseSetupCfg(string(content))

	// Always mark this as the metadata source so downstream consumers
	// can see that setup.cfg was inspected even when individual fields
	// are absent.
	metadata.LanguageSpecific["metadata_source"] = "setup.cfg"
	metadata.VersionSource = "setup.cfg"

	classifiers := applySetupCfgCoreMetadata(metadata, cfg)

	if err := applySetupCfgPythonMatrix(metadata, cfg, classifiers); err != nil {
		return err
	}

	// install_requires: multi-line list
	if deps := setupCfgLines(cfg, "options", "install_requires"); len(deps) > 0 {
		metadata.LanguageSpecific["dependencies"] = deps
		metadata.LanguageSpecific["dependency_count"] = len(deps)
		metadata.LanguageSpecific["dependencies_source"] = "setup.cfg"
	}

	applySetupCfgVersioning(metadata, cfg)

	if metadata.Name != "" {
		packageName := strings.ReplaceAll(metadata.Name, "-", "_")
		projectMatchPackage := metadata.Name == packageName
		metadata.LanguageSpecific["project_match_package"] = projectMatchPackage
	}

	return nil
}

// setupCfgScalar returns the trimmed raw value of a setup.cfg key, or an
// empty string when the section/key is absent.
func setupCfgScalar(cfg map[string]map[string]setupCfgValue, section, key string) string {
	if s, ok := cfg[section]; ok {
		if v, ok := s[key]; ok {
			return strings.TrimSpace(v.Raw)
		}
	}
	return ""
}

// setupCfgLines returns the per-line split of a setup.cfg key's value, or
// nil when the section/key is absent.
func setupCfgLines(cfg map[string]map[string]setupCfgValue, section, key string) []string {
	if s, ok := cfg[section]; ok {
		if v, ok := s[key]; ok {
			return v.Lines
		}
	}
	return nil
}

// applySetupCfgCoreMetadata copies the [metadata] scalar fields onto the
// shared struct and returns the classifier list (checked under both the
// canonical `classifiers` key and the older singular `classifier`) so the
// caller can reuse it for classifier-derived matrix generation.
func applySetupCfgCoreMetadata(metadata *extractor.ProjectMetadata, cfg map[string]map[string]setupCfgValue) []string {
	metadata.Name = setupCfgScalar(cfg, "metadata", "name")
	metadata.Version = setupCfgScalar(cfg, "metadata", "version")
	metadata.Description = setupCfgScalar(cfg, "metadata", "description")
	metadata.License = setupCfgScalar(cfg, "metadata", "license")
	metadata.Homepage = setupCfgScalar(cfg, "metadata", "url")
	if metadata.Homepage == "" {
		metadata.Homepage = setupCfgScalar(cfg, "metadata", "home_page")
	}

	if author := setupCfgScalar(cfg, "metadata", "author"); author != "" {
		email := setupCfgScalar(cfg, "metadata", "author_email")
		if email != "" {
			metadata.Authors = []string{fmt.Sprintf("%s <%s>", author, email)}
		} else {
			metadata.Authors = []string{author}
		}
	}

	metadata.LanguageSpecific["package_name"] = metadata.Name

	// Classifiers are multi-line by convention; the parser splits them
	// into one line per entry already.
	classifiers := setupCfgLines(cfg, "metadata", "classifiers")
	if len(classifiers) == 0 {
		// Older PBR/setuptools spelt this as the singular form.
		classifiers = setupCfgLines(cfg, "metadata", "classifier")
	}
	if len(classifiers) > 0 {
		metadata.LanguageSpecific["classifiers"] = classifiers
	}

	if kw := setupCfgScalar(cfg, "metadata", "keywords"); kw != "" {
		metadata.LanguageSpecific["keywords"] = kw
	}

	return classifiers
}

// applySetupCfgPythonMatrix emits the Python version matrix from
// python_requires (in either [metadata] or [options]); when that is
// absent it derives the matrix from explicit Python trove classifiers.
func applySetupCfgPythonMatrix(metadata *extractor.ProjectMetadata, cfg map[string]map[string]setupCfgValue, classifiers []string) error {
	// python_requires can appear in either [metadata] or [options]; the
	// parser has already normalised the hyphenated form (`python-requires`)
	// to underscores so we only need to check one spelling.
	pythonRequires := setupCfgScalar(cfg, "metadata", "python_requires")
	if pythonRequires == "" {
		pythonRequires = setupCfgScalar(cfg, "options", "python_requires")
	}
	if pythonRequires != "" {
		metadata.LanguageSpecific["requires_python"] = pythonRequires
		return resolveAndEmitMatrix(metadata, pythonRequires, "requires-python")
	}

	// No python_requires declared but classifiers contain explicit
	// Python version markers. Treat those as the authoritative matrix.
	classifierVersions := derivePythonVersionsFromClassifiers(classifiers)
	if len(classifierVersions) > 0 {
		metadata.LanguageSpecific["version_matrix"] = classifierVersions
		metadata.LanguageSpecific["matrix_json"] = fmt.Sprintf(`{"python-version": [%s]}`,
			strings.Join(quoteStrings(classifierVersions), ", "))
		metadata.LanguageSpecific["build_version"] = classifierVersions[len(classifierVersions)-1]
		metadata.LanguageSpecific["requires_python_source"] = "classifiers"
		emitEOLOutputs(metadata, classifierVersions)
	}
	return nil
}

// applySetupCfgVersioning determines the versioning type and, when
// dynamic, the provider responsible and whether the version is
// unresolved at extraction time.
func applySetupCfgVersioning(metadata *extractor.ProjectMetadata, cfg map[string]map[string]setupCfgValue) {
	provider := detectDynamicProviderFromSetupCfg(cfg)
	if provider == "" {
		metadata.LanguageSpecific["versioning_type"] = "static"
		return
	}

	metadata.LanguageSpecific["versioning_type"] = "dynamic"
	metadata.LanguageSpecific["dynamic_provider"] = provider
	// Any dynamic provider that hasn't already produced a concrete
	// version string is, by definition, unresolved at extraction time
	// (PBR/setuptools-scm/versioneer/runtime-attr all defer to build).
	if strings.TrimSpace(metadata.Version) == "" {
		metadata.LanguageSpecific["version_unresolved"] = true
	}
	// `version = attr:` / `file:` are indirections that only resolve
	// at build-time. Stash the raw expression for diagnostics and
	// clear the surface Version so it doesn't pollute outputs like
	// project_version with a non-resolvable literal.
	if provider == "setuptools-dynamic" {
		if rawVer := strings.TrimSpace(metadata.Version); rawVer != "" {
			if strings.HasPrefix(rawVer, "attr:") || strings.HasPrefix(rawVer, "file:") {
				metadata.LanguageSpecific["version_expression"] = rawVer
				metadata.Version = ""
				metadata.LanguageSpecific["version_unresolved"] = true
			}
		}
	}
}

// setupCfgValue represents a value parsed from setup.cfg. Python's
// RawConfigParser folds indented continuation lines onto the preceding
// key; multi-line values are typically intended as lists. We retain both
// the raw scalar (joined with newlines, trimmed) and the per-line split
// for convenience.
type setupCfgValue struct {
	Raw   string
	Lines []string
}

// setupCfgParser carries the mutable state used while walking a setup.cfg
// document line by line.
type setupCfgParser struct {
	result         map[string]map[string]setupCfgValue
	currentSection string
	currentKey     string
	currentValue   []string
}

// flush commits the accumulated value for the current key. Multi-line
// values are also split into non-empty trimmed lines for list callers.
func (p *setupCfgParser) flush() {
	if p.currentSection == "" || p.currentKey == "" {
		return
	}
	raw := strings.TrimSpace(strings.Join(p.currentValue, "\n"))
	var lines []string
	if raw != "" {
		for _, l := range strings.Split(raw, "\n") {
			l = strings.TrimSpace(l)
			if l != "" {
				lines = append(lines, l)
			}
		}
	}
	p.result[p.currentSection][p.currentKey] = setupCfgValue{Raw: raw, Lines: lines}
}

// feed processes a single raw line, updating parser state per Python
// configparser semantics (continuation folding, section headers, comment
// handling, and `=`/`:` separated key-value pairs).
func (p *setupCfgParser) feed(rawLine string) {
	line := strings.TrimRight(rawLine, "\r")
	trimmed := strings.TrimSpace(line)
	isIndented := len(line) > 0 && (line[0] == ' ' || line[0] == '\t')

	// Indented (continuation) whitespace-only lines are preserved as
	// empty entries rather than terminating the value, matching
	// Python's RawConfigParser semantics.
	if trimmed == "" && isIndented && p.currentKey != "" {
		p.currentValue = append(p.currentValue, "")
		return
	}

	// A fully blank (unindented) line terminates the current value.
	if trimmed == "" {
		p.flush()
		p.currentKey = ""
		p.currentValue = nil
		return
	}

	// Full-line comments (configparser treats inline `;` as part of
	// the value, so only handle leading-character comments here).
	if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
		return
	}

	// Section header
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		p.flush()
		// Trim brackets first, then strip any internal whitespace so
		// inputs like `[ metadata ]` normalise to `metadata` (matching
		// Python's configparser behaviour).
		p.currentSection = strings.ToLower(strings.TrimSpace(strings.Trim(trimmed, "[]")))
		if _, ok := p.result[p.currentSection]; !ok {
			p.result[p.currentSection] = make(map[string]setupCfgValue)
		}
		p.currentKey = ""
		p.currentValue = nil
		return
	}

	// Continuation line: starts with whitespace and we already have a key
	if isIndented && p.currentKey != "" {
		p.currentValue = append(p.currentValue, trimmed)
		return
	}

	if p.currentSection == "" {
		return
	}

	sep := setupCfgSeparatorIndex(trimmed)
	if sep < 0 {
		return
	}

	p.flush()
	key := strings.TrimSpace(trimmed[:sep])
	val := strings.TrimSpace(trimmed[sep+1:])
	p.currentKey = strings.ReplaceAll(strings.ToLower(key), "-", "_")
	p.currentValue = []string{val}
}

// setupCfgSeparatorIndex returns the index of the key/value separator in
// a trimmed line, preferring whichever of `=` or `:` appears first, or -1
// when neither is present. Matches Python configparser, which accepts
// both separators.
func setupCfgSeparatorIndex(trimmed string) int {
	if idx := strings.Index(trimmed, "="); idx >= 0 {
		sep := idx
		if jdx := strings.Index(trimmed, ":"); jdx >= 0 && jdx < idx {
			sep = jdx
		}
		return sep
	}
	if idx := strings.Index(trimmed, ":"); idx >= 0 {
		return idx
	}
	return -1
}

// parseSetupCfg parses a setup.cfg file using continuation-aware INI
// rules. Section and key names are lowercased and hyphens normalised to
// underscores so callers can look up canonical names regardless of
// whether the file used `author-email` (older setuptools / PBR) or
// `author_email` (canonical) styles. Both `=` and `:` separators are
// accepted, matching Python's configparser.
func parseSetupCfg(content string) map[string]map[string]setupCfgValue {
	p := &setupCfgParser{result: make(map[string]map[string]setupCfgValue)}
	for _, rawLine := range strings.Split(content, "\n") {
		p.feed(rawLine)
	}
	p.flush()
	return p.result
}

// detectDynamicProviderFromSetupCfg returns the name of the dynamic
// versioning provider in use, or empty string if the project's version is
// static. Recognised providers: "pbr", "setuptools-scm", "versioneer",
// "setuptools-dynamic".
func detectDynamicProviderFromSetupCfg(cfg map[string]map[string]setupCfgValue) string {
	if _, ok := cfg["pbr"]; ok {
		return "pbr"
	}
	if meta, ok := cfg["metadata"]; ok {
		if v, ok := meta["version"]; ok {
			s := strings.TrimSpace(v.Raw)
			if strings.HasPrefix(s, "attr:") || strings.HasPrefix(s, "file:") {
				return "setuptools-dynamic"
			}
		}
	}
	if opts, ok := cfg["options"]; ok {
		if v, ok := opts["setup_requires"]; ok {
			for _, line := range v.Lines {
				name := extractRequirementName(line)
				switch name {
				case "pbr":
					return "pbr"
				case "setuptools_scm", "setuptools-scm":
					return "setuptools-scm"
				case "versioneer":
					return "versioneer"
				}
			}
		}
	}
	return ""
}

// extractRequirementName returns the lowercased distribution name token
// at the start of a PEP 508 requirement line (e.g. `pbr>=2.0 ; ...` ->
// `pbr`). Returns an empty string when no valid name is found. This is
// deliberately stricter than substring matching so that requirements
// such as `sphinx-pbr-theme` do not get mistaken for `pbr`.
func extractRequirementName(line string) string {
	s := strings.TrimSpace(line)
	// Drop common surrounding punctuation left over from list/quoted forms.
	s = strings.Trim(s, "'\",[]() \t")
	nameRe := regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*`)
	match := nameRe.FindString(s)
	return strings.ToLower(match)
}

// parseINI parses a simple INI file into a map of sections
func parseINI(content string) map[string]map[string]string {
	result := make(map[string]map[string]string)
	var currentSection string

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		// Section header
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.ToLower(strings.Trim(line, "[]"))
			result[currentSection] = make(map[string]string)
			continue
		}

		// Key-value pair
		if currentSection != "" && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				result[currentSection][key] = value
			}
		}
	}

	return result
}
