// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package python

import (
	"fmt"
	"os"
	"regexp"
	"slices"
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

	call := setupCallKeywords(text)
	metadata.Name = strings.TrimSpace(call["name"])
	metadata.Version = strings.TrimSpace(call["version"])
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
		if classifierVersions, eol := derivePythonVersionsFromClassifiers(classifiers); len(classifierVersions) > 0 {
			metadata.LanguageSpecific["version_matrix"] = classifierVersions
			metadata.LanguageSpecific["matrix_json"] = fmt.Sprintf(`{"python-version": [%s]}`,
				strings.Join(quoteStrings(classifierVersions), ", "))
			metadata.LanguageSpecific["build_version"] = classifierVersions[len(classifierVersions)-1]
			metadata.LanguageSpecific["requires_python_source"] = "classifiers"
			emitEOLOutputs(metadata, classifierVersions, eol)
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

// detectDynamicProviderFromSetupPy returns the dynamic versioning
// provider that the keyword arguments of a setup.py's setup() call
// name, or "" when they name none. The call is the one
// setupCallKeywords reads; a comment, a string, an assignment or
// another call never counts. When several providers are named, the
// earliest of pbr, setuptools-scm, versioneer and runtime-attr wins.
func detectDynamicProviderFromSetupPy(text string) string {
	args := map[string][]pyToken{}
	for _, kw := range setupCallArguments(text) {
		if _, seen := args[kw.name]; !seen {
			args[kw.name] = kw.value
		}
	}

	requires := map[string]bool{}
	for _, name := range setupRequiresNames(args["setup_requires"]) {
		requires[name] = true
	}
	version, cmdclass := args["version"], args["cmdclass"]

	switch {
	case isPyName(args["pbr"], "True") || requires["pbr"]:
		return "pbr"
	case scmVersionEnabled(args["use_scm_version"]) || requires["setuptools-scm"] || requires["setuptools_scm"]:
		return "setuptools-scm"
	case requires["versioneer"] ||
		callsAttribute(version, "versioneer", "get_version", "get_cmdclass") ||
		callsAttribute(cmdclass, "versioneer", "get_version", "get_cmdclass"):
		return "versioneer"
	case startsWithName(version) &&
		(holdsDunderVersion(version) || callsName(version, "get_version", "read_version")):
		return "runtime-attr"
	}
	return ""
}

// enclosesAll reports whether the bracket opening value closes at its
// last token and not before.
func enclosesAll(value []pyToken) bool {
	depth := 0
	for i, tok := range value {
		depth += bracketDelta(tok)
		if depth == 0 {
			return i == len(value)-1
		}
	}
	return false
}

func isPyName(value []pyToken, name string) bool {
	return len(value) == 1 && value[0].kind == pyName && value[0].text == name
}

// pyKeywords holds Python's reserved keywords. None of them is a name
// in the sense startsWithName reads.
var pyKeywords = map[string]bool{
	"False": true, "None": true, "True": true, "and": true, "as": true, "assert": true,
	"async": true, "await": true, "break": true, "class": true, "continue": true, "def": true,
	"del": true, "elif": true, "else": true, "except": true, "finally": true, "for": true,
	"from": true, "global": true, "if": true, "import": true, "in": true, "is": true,
	"lambda": true, "nonlocal": true, "not": true, "or": true, "pass": true, "raise": true,
	"return": true, "try": true, "while": true, "with": true, "yield": true,
}

// isPyIdentifier reports whether tok is a name that is not a keyword.
func isPyIdentifier(tok pyToken) bool {
	return tok.kind == pyName && !pyKeywords[tok.text]
}

// startsWithName reports whether value starts with a name that is not
// a Python keyword. A value is judged by how it starts, never by what
// it would evaluate to.
func startsWithName(value []pyToken) bool {
	return len(value) > 0 && isPyIdentifier(value[0])
}

// scmVersionEnabled reports whether a use_scm_version value turns
// setuptools-scm on: True, a { ... } display that is not empty, or a
// value that starts with a name, such as a variable or a call. Any
// other value, such as a number, a string, a list or one that starts
// with a keyword like not, gives no signal.
func scmVersionEnabled(value []pyToken) bool {
	if isPyName(value, "True") {
		return true
	}
	if len(value) > 2 && value[0].text+value[len(value)-1].text == "{}" && enclosesAll(value) {
		return true
	}
	return startsWithName(value)
}

// setupRequiresNames returns the lowercased distribution names of the
// string literals in a setup_requires value written as a list. Each
// item counts only when it is one plain string literal; a list built
// any other way, or a variable, gives no names.
func setupRequiresNames(value []pyToken) []string {
	if len(value) < 2 || value[0].text != "[" || value[len(value)-1].text != "]" || !enclosesAll(value) {
		return nil
	}
	var names []string
	addItem := func(item []pyToken) {
		if len(item) != 1 || item[0].kind != pyString || !item[0].plain || strings.Contains(item[0].value, "\\") {
			return
		}
		if name := extractRequirementName(item[0].value); name != "" {
			names = append(names, name)
		}
	}
	inner := value[1 : len(value)-1]
	depth, start := 0, 0
	for i, tok := range inner {
		if depth == 0 && tok.text == "," && tok.kind == pyOther {
			addItem(inner[start:i])
			start = i + 1
			continue
		}
		depth += bracketDelta(tok)
	}
	addItem(inner[start:])
	return names
}

// callsAttribute reports whether value calls <object>.<attr>(...) for
// one of attrs.
func callsAttribute(value []pyToken, object string, attrs ...string) bool {
	for i := 0; i+3 < len(value); i++ {
		if value[i].kind == pyName && value[i].text == object && value[i+1].text == "." &&
			value[i+2].kind == pyName && slices.Contains(attrs, value[i+2].text) && value[i+3].text == "(" {
			return true
		}
	}
	return false
}

// callsName reports whether value calls a function, plain or as an
// attribute, named one of names.
func callsName(value []pyToken, names ...string) bool {
	for i := 0; i+1 < len(value); i++ {
		if value[i].kind == pyName && slices.Contains(names, value[i].text) && value[i+1].text == "(" {
			return true
		}
	}
	return false
}

// holdsDunderVersion reports whether value reads __version__, as a
// name, an attribute or the key of a subscript such as
// about["__version__"]. A __version__ that labels a keyword argument,
// and the string "__version__" anywhere but a subscript key, are
// declined.
func holdsDunderVersion(value []pyToken) bool {
	for i, tok := range value {
		if tok.kind == pyName && tok.text == "__version__" && (i+1 >= len(value) || value[i+1].text != "=") {
			return true
		}
		if tok.kind == pyString && tok.value == "__version__" && isSubscriptKey(value, i) {
			return true
		}
	}
	return false
}

// isSubscriptKey reports whether value[i] is the whole key of a
// subscript on a name that is not a keyword, a call or another
// subscript, as in about["__version__"].
func isSubscriptKey(value []pyToken, i int) bool {
	if i < 2 || i+1 >= len(value) || value[i-1].text != "[" || value[i+1].text != "]" {
		return false
	}
	prev := value[i-2]
	return isPyIdentifier(prev) || (prev.kind == pyOther && (prev.text == ")" || prev.text == "]"))
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
