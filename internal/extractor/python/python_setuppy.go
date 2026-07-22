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

// extractFromSetupPy extracts metadata from setup.py using regex patterns
func (e *Extractor) extractFromSetupPy(path string, metadata *extractor.ProjectMetadata) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read setup.py: %w", err)
	}

	text := string(content)

	metadata.Name = extractSetupPyField(text, "name")
	metadata.Version = extractSetupPyField(text, "version")
	metadata.Description = extractSetupPyField(text, "description")
	metadata.License = extractSetupPyField(text, "license")
	metadata.Homepage = extractSetupPyField(text, "url")
	metadata.VersionSource = "setup.py"

	if author := extractSetupPyField(text, "author"); author != "" {
		email := extractSetupPyField(text, "author_email")
		if email != "" {
			metadata.Authors = []string{fmt.Sprintf("%s <%s>", author, email)}
		} else {
			metadata.Authors = []string{author}
		}
	}

	// Python-specific
	metadata.LanguageSpecific["package_name"] = metadata.Name
	metadata.LanguageSpecific["metadata_source"] = "setup.py"

	// Surface install_requires when declared inline, so the Extract()
	// loop knows not to fall back to requirements.txt for projects that
	// already provide an explicit dependency list in setup.py.
	if deps := extractSetupPyInstallRequires(text); len(deps) > 0 {
		metadata.LanguageSpecific["dependencies"] = deps
		metadata.LanguageSpecific["dependency_count"] = len(deps)
		metadata.LanguageSpecific["dependencies_source"] = "setup.py"
	}

	if pythonRequires := extractSetupPyField(text, "python_requires"); pythonRequires != "" {
		metadata.LanguageSpecific["requires_python"] = pythonRequires
		if err := resolveAndEmitMatrix(metadata, pythonRequires, "requires-python"); err != nil {
			return err
		}
	} else if classifiers := extractSetupPyClassifiers(text); len(classifiers) > 0 {
		metadata.LanguageSpecific["classifiers"] = classifiers
		if classifierVersions := derivePythonVersionsFromClassifiers(classifiers); len(classifierVersions) > 0 {
			metadata.LanguageSpecific["version_matrix"] = classifierVersions
			metadata.LanguageSpecific["matrix_json"] = fmt.Sprintf(`{"python-version": [%s]}`,
				strings.Join(quoteStrings(classifierVersions), ", "))
			metadata.LanguageSpecific["build_version"] = classifierVersions[len(classifierVersions)-1]
			metadata.LanguageSpecific["requires_python_source"] = "classifiers"
			emitEOLOutputs(metadata, classifierVersions)
		}
	}

	// Determine versioning type and (if dynamic) the provider responsible.
	provider := detectDynamicProviderFromSetupPy(text)
	if provider != "" {
		metadata.LanguageSpecific["versioning_type"] = "dynamic"
		metadata.LanguageSpecific["dynamic_provider"] = provider
		// All dynamic providers resolve the real version at build time;
		// surface that as `version_unresolved` whenever extraction did
		// not turn up a concrete value (matches setup.cfg behaviour).
		if strings.TrimSpace(metadata.Version) == "" {
			metadata.LanguageSpecific["version_unresolved"] = true
		}
	} else {
		metadata.LanguageSpecific["versioning_type"] = "static"
	}

	// Compare project name and package name
	if metadata.Name != "" {
		// Package name is project name with dashes replaced by underscores
		packageName := strings.ReplaceAll(metadata.Name, "-", "_")
		projectMatchPackage := metadata.Name == packageName
		metadata.LanguageSpecific["project_match_package"] = projectMatchPackage
	}

	return nil
}

// crossCheckDynamicFromSetupPy reads a sibling setup.py and, if it
// reveals a dynamic versioning provider that the setup.cfg analysis did
// not surface, upgrades the metadata accordingly. This is the canonical
// PBR layout (declarative setup.cfg + minimal setup.py shim).
func crossCheckDynamicFromSetupPy(setupPyPath string, metadata *extractor.ProjectMetadata) {
	if metadata == nil || metadata.LanguageSpecific == nil {
		return
	}
	if provider, _ := metadata.LanguageSpecific["dynamic_provider"].(string); provider != "" {
		return // already determined from setup.cfg
	}
	content, err := os.ReadFile(setupPyPath)
	if err != nil {
		return
	}
	provider := detectDynamicProviderFromSetupPy(string(content))
	if provider == "" {
		return
	}
	metadata.LanguageSpecific["versioning_type"] = "dynamic"
	metadata.LanguageSpecific["dynamic_provider"] = provider
	if strings.TrimSpace(metadata.Version) == "" {
		metadata.LanguageSpecific["version_unresolved"] = true
	}
}

// extractSetupRequiresNames returns the lowercased distribution names of
// every requirement listed in a `setup_requires=[...]` keyword argument
// inside a setup.py source. Each requirement is parsed via
// `extractRequirementName` so that PEP 508 version specifiers, environment
// markers, and extras are stripped before name matching. Returns an empty
// slice when no `setup_requires=[...]` is found.
func extractSetupRequiresNames(text string) []string {
	listRe := regexp.MustCompile(`(?s)setup_requires\s*=\s*\[([^\]]*)\]`)
	itemRe := regexp.MustCompile(`['"]([^'"]+)['"]`)
	var names []string
	for _, list := range listRe.FindAllStringSubmatch(text, -1) {
		for _, item := range itemRe.FindAllStringSubmatch(list[1], -1) {
			if name := extractRequirementName(item[1]); name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}

// detectDynamicProviderFromSetupPy returns the name of the dynamic
// versioning provider implied by a setup.py file, or empty string if the
// version appears static. The heuristics deliberately err on the side of
// recognising PBR/setuptools-scm/versioneer rather than the previous
// substring checks which only matched a handful of helper-function names.
func detectDynamicProviderFromSetupPy(text string) string {
	lower := strings.ToLower(text)

	// Parse the setup_requires=[...] list (if any) and extract the
	// distribution name for each entry. Matching on the parsed name
	// (rather than on a regex prefix inside the list) avoids treating
	// unrelated packages such as `sphinx-pbr-theme` or
	// `setuptools_scm_git_archive` as PBR / setuptools-scm providers.
	setupRequiresNames := extractSetupRequiresNames(text)
	hasPbrRequirement := false
	hasScmRequirement := false
	hasVersioneerRequirement := false
	for _, name := range setupRequiresNames {
		switch name {
		case "pbr":
			hasPbrRequirement = true
		case "setuptools-scm", "setuptools_scm":
			hasScmRequirement = true
		case "versioneer":
			hasVersioneerRequirement = true
		}
	}

	// `pbr=True` keyword argument to setup(...). Use word boundaries to
	// avoid matching unrelated identifiers ending in `pbr`.
	pbrKwarg := regexp.MustCompile(`\bpbr\s*=\s*true\b`).MatchString(lower)

	if pbrKwarg || hasPbrRequirement {
		return "pbr"
	}
	if strings.Contains(lower, "use_scm_version") || hasScmRequirement {
		return "setuptools-scm"
	}
	if hasVersioneerRequirement ||
		strings.Contains(lower, "versioneer.get_version") ||
		strings.Contains(lower, "versioneer.get_cmdclass") {
		return "versioneer"
	}
	// Legacy helper-call patterns (kept from the original implementation)
	if strings.Contains(text, "__version__") ||
		strings.Contains(text, "version=get_version") ||
		strings.Contains(text, "version=read_version") {
		return "runtime-attr"
	}
	return ""
}

// extractSetupPyInstallRequires returns the list of install_requires entries
// declared in a setup.py file. It handles single- and double-quoted entries
// inside the `install_requires=[...]` keyword argument. Empty/whitespace
// items are skipped. Returns nil when the keyword is absent.
func extractSetupPyInstallRequires(content string) []string {
	listRe := regexp.MustCompile(`(?s)install_requires\s*=\s*\[(.*?)\]`)
	listMatch := listRe.FindStringSubmatch(content)
	if len(listMatch) < 2 {
		return nil
	}
	itemRe := regexp.MustCompile(`['"]([^'"]+)['"]`)
	items := itemRe.FindAllStringSubmatch(listMatch[1], -1)
	var result []string
	for _, m := range items {
		if len(m) > 1 {
			if v := strings.TrimSpace(m[1]); v != "" {
				result = append(result, v)
			}
		}
	}
	return result
}

// extractSetupPyClassifiers returns the list of trove classifier strings
// declared in a setup.py file. It handles single- and double-quoted
// entries inside the `classifiers=[...]` keyword argument.
func extractSetupPyClassifiers(content string) []string {
	listRe := regexp.MustCompile(`(?s)classifiers\s*=\s*\[(.*?)\]`)
	listMatch := listRe.FindStringSubmatch(content)
	if len(listMatch) < 2 {
		return nil
	}
	itemRe := regexp.MustCompile(`['"]([^'"]+)['"]`)
	items := itemRe.FindAllStringSubmatch(listMatch[1], -1)
	var result []string
	for _, m := range items {
		if len(m) > 1 {
			result = append(result, m[1])
		}
	}
	return result
}

// extractSetupPyField extracts a field value from setup.py using regex
func extractSetupPyField(content, field string) string {
	// Pattern: field='value' or field="value" or field='''value''' or field="""value"""
	patterns := []string{
		fmt.Sprintf(`%s\s*=\s*['"]([^'"]+)['"]`, field),
		fmt.Sprintf(`%s\s*=\s*'''([^']+)'''`, field),
		fmt.Sprintf(`%s\s*=\s*"""([^"]+)"""`, field),
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(content); len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
	}

	return ""
}
