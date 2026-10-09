// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package javascript

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// The package manager names this extractor reports. Yarn is split in two
// because Yarn Berry (2 and later) differs from Yarn classic in its
// commands, its lockfile and how it is installed.
const (
	managerNpm       = "npm"
	managerYarn      = "yarn"
	managerYarnBerry = "yarn-berry"
	managerPnpm      = "pnpm"
	managerBun       = "bun"
)

// lockFile pairs a lockfile name with the package manager that writes it.
// The manager is the family, so Yarn classic and Berry share "yarn".
type lockFile struct {
	name    string
	manager string
}

// knownLockFiles lists every lockfile in detection priority: pnpm, then
// Yarn, then npm, then Bun. Within a manager the file the tool itself
// prefers comes first: npm reads npm-shrinkwrap.json over
// package-lock.json, and Bun 1.2 replaced the binary bun.lockb with the
// text bun.lock.
var knownLockFiles = []lockFile{
	{"pnpm-lock.yaml", managerPnpm},
	{"yarn.lock", managerYarn},
	{"npm-shrinkwrap.json", managerNpm},
	{"package-lock.json", managerNpm},
	{"bun.lock", managerBun},
	{"bun.lockb", managerBun},
}

// reportableVersion limits the version output to characters a version or
// semver range needs. Callers pass it to Corepack, often from a shell, so
// anything else (a URL, a space, shell syntax, glob characters such as *)
// is withheld. Wildcard ranges remain expressible as 10.x. A reported
// version must also parse as a version or range, which rules out tags
// such as "latest".
var reportableVersion = regexp.MustCompile(`^[0-9A-Za-z.^~-]+$`)

// corepackHash matches the integrity hash Corepack appends to a version,
// as in "4.5.0+sha512.<hex>". Other build metadata is left in place.
var corepackHash = regexp.MustCompile(`\+sha[0-9]+\.[0-9A-Fa-f]+$`)

// declaration is a package manager that package.json names, in either the
// packageManager field or devEngines.packageManager.
type declaration struct {
	manager string // family: npm, yarn, pnpm or bun
	version string // as written, minus any Corepack integrity hash
	source  string // the field it came from, for warnings
}

// packageManager is the detection result.
type packageManager struct {
	name     string // npm, yarn, yarn-berry, pnpm or bun
	version  string // from the declaration; empty when none is reportable
	lockFile string // lockfile of that manager; empty when none exists
}

// detectPackageManager resolves the package manager from the package.json
// declarations and the lockfiles present, along with warnings describing
// any disagreement between them. A declaration wins over lockfiles, and
// lockfiles follow knownLockFiles priority; npm is the default.
func detectPackageManager(projectPath string, pkg *PackageJSON) (packageManager, []string) {
	present := presentLockFiles(projectPath)
	declared, warnings := declaredPackageManager(projectPath, pkg, present)

	manager := managerNpm
	switch {
	case declared.manager != "":
		manager = declared.manager
	case len(present) > 0:
		manager = present[0].manager
	}

	result := packageManager{name: manager}
	if manager == managerYarn {
		result.name = yarnVariant(projectPath, declared.version)
	}
	for _, lock := range present {
		if lock.manager == manager {
			result.lockFile = lock.name
			break
		}
	}

	if declared.version != "" {
		if _, parses := admittedMajors(declared.version); parses && reportableVersion.MatchString(declared.version) {
			result.version = declared.version
		} else {
			warnings = append(warnings, declared.source+" has a version this action cannot report; omitting it")
		}
	}

	return result, append(warnings, lockFileWarnings(declared, present, result.name)...)
}

// declaredPackageManager returns the package manager package.json names.
// The packageManager field takes precedence over devEngines.packageManager,
// matching Corepack. devEngines may list alternatives, any of which npm
// accepts; without the field, the one matching the lockfiles present (in
// knownLockFiles priority) stands for the project, else the first.
func declaredPackageManager(projectPath string, pkg *PackageJSON, present []lockFile) (declaration, []string) {
	field, warnings := parsePackageManagerField(pkg.PackageManager)
	alternatives, engineWarnings := parseDevEngines(pkg.DevEngines)
	warnings = append(warnings, engineWarnings...)

	if field.manager == "" {
		return chooseAlternative(projectPath, alternatives, present), warnings
	}
	if len(alternatives) == 0 {
		return field, warnings
	}

	var matching []declaration
	for _, alternative := range alternatives {
		if alternative.manager == field.manager {
			matching = append(matching, alternative)
		}
	}
	if len(matching) == 0 {
		warnings = append(warnings, fmt.Sprintf(
			"packageManager names %s, which devEngines.packageManager does not list; using %s",
			field.manager, field.manager))
		return field, warnings
	}

	// Corepack requires the packageManager version to satisfy the
	// devEngines range. Warn when no matching alternative admits its major;
	// an alternative this cannot parse is given the benefit of the doubt.
	fieldRanges, ok := admittedMajors(field.version)
	if !ok {
		return field, warnings
	}
	fieldMajor, ok := singleMajor(fieldRanges)
	if !ok {
		return field, warnings
	}
	for _, alternative := range matching {
		ranges, ok := admittedMajors(alternative.version)
		if !ok || admitsMajor(ranges, fieldMajor) {
			return field, warnings
		}
	}
	warnings = append(warnings, fmt.Sprintf(
		"packageManager names %s %d, which devEngines.packageManager does not allow; using %s %d",
		field.manager, fieldMajor, field.manager, fieldMajor))
	return field, warnings
}

// chooseAlternative picks the devEngines alternative matching the first
// lockfile present that any alternative matches, else the first one.
// Yarn classic and Berry share yarn.lock, so among Yarn alternatives it
// prefers one admitting the generation the project's files indicate,
// passing over versions it cannot parse.
func chooseAlternative(projectPath string, alternatives []declaration, present []lockFile) declaration {
	if len(alternatives) == 0 {
		return declaration{}
	}
	for _, lock := range present {
		var candidates []declaration
		for _, alternative := range alternatives {
			if alternative.manager == lock.manager {
				candidates = append(candidates, alternative)
			}
		}
		if len(candidates) == 0 {
			continue
		}
		if lock.manager == managerYarn {
			generation := yarnFromFiles(projectPath)
			for _, candidate := range candidates {
				classic, berry, ok := yarnGenerations(candidate.version)
				if ok && ((generation == managerYarnBerry && berry) || (generation == managerYarn && classic)) {
					return candidate
				}
			}
		}
		return candidates[0]
	}
	return alternatives[0]
}

// parsePackageManagerField parses Corepack's "name@version+hash" form.
func parsePackageManagerField(value string) (declaration, []string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return declaration{}, nil
	}
	name, version, _ := strings.Cut(value, "@")
	return newDeclaration("packageManager", name, version)
}

// devEngine is one devEngines.packageManager entry.
type devEngine struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// parseDevEngines reads devEngines.packageManager, which is an object or
// an array of alternative objects, returning the valid entries in order.
func parseDevEngines(raw json.RawMessage) ([]declaration, []string) {
	var engines struct {
		PackageManager json.RawMessage `json:"packageManager"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &engines) != nil || len(engines.PackageManager) == 0 {
		return nil, nil
	}

	const source = "devEngines.packageManager"
	var entries []devEngine
	var entry devEngine
	if err := json.Unmarshal(engines.PackageManager, &entry); err == nil {
		entries = []devEngine{entry}
	} else if err := json.Unmarshal(engines.PackageManager, &entries); err != nil || len(entries) == 0 {
		return nil, []string{source + " is neither an object nor a list of objects; ignoring it"}
	}

	var alternatives []declaration
	var warnings []string
	for _, entry := range entries {
		alternative, entryWarnings := newDeclaration(source, entry.Name, entry.Version)
		warnings = append(warnings, entryWarnings...)
		if alternative.manager != "" {
			alternatives = append(alternatives, alternative)
		}
	}
	if len(alternatives) > 0 {
		// An unsupported alternative is harmless beside a supported one.
		return alternatives, nil
	}
	return nil, warnings
}

// newDeclaration validates a declared package manager name and strips a
// trailing Corepack integrity hash ("+sha512.<hex>") from the version;
// both fields may carry one. Warnings never repeat the declared values,
// which callers do not control.
func newDeclaration(source, name, version string) (declaration, []string) {
	version = corepackHash.ReplaceAllString(strings.TrimSpace(version), "")
	manager := strings.ToLower(strings.TrimSpace(name))
	switch manager {
	case managerNpm, managerYarn, managerPnpm, managerBun:
		return declaration{manager: manager, version: version, source: source}, nil
	case "":
		return declaration{}, []string{source + " names no package manager; ignoring it"}
	default:
		return declaration{}, []string{source + " names an unsupported package manager; ignoring it"}
	}
}

// presentLockFiles returns the known lockfiles in projectPath, in
// knownLockFiles order.
func presentLockFiles(projectPath string) []lockFile {
	var present []lockFile
	for _, lock := range knownLockFiles {
		if isFile(filepath.Join(projectPath, lock.name)) {
			present = append(present, lock)
		}
	}
	return present
}

// yarnVariant tells Yarn Berry from Yarn classic. A declared version or
// range decides when it admits releases of one generation only, classic
// (below 2) or Berry (2 and later). Otherwise, as with no version or a
// range such as ">=1" spanning both, the project's files decide.
func yarnVariant(projectPath, version string) string {
	if classic, berry, ok := yarnGenerations(version); ok && classic != berry {
		if berry {
			return managerYarnBerry
		}
		return managerYarn
	}
	return yarnFromFiles(projectPath)
}

// yarnGenerations reports whether a Yarn version or range admits classic
// releases (below 2) and Berry releases (2 and later). It reports false
// when the range cannot be parsed.
func yarnGenerations(version string) (classic, berry, ok bool) {
	ranges, ok := admittedMajors(version)
	if !ok {
		return false, false, false
	}
	for _, r := range ranges {
		classic = classic || r.lo < 2
		berry = berry || r.hi >= 2
	}
	return classic, berry, true
}

// yarnFromFiles reads the Yarn generation from the project's files: a
// .yarnrc.yml (Berry's configuration file) or a Berry lockfile means
// Berry, and anything else classic.
func yarnFromFiles(projectPath string) string {
	if isFile(filepath.Join(projectPath, ".yarnrc.yml")) ||
		isBerryLockFile(filepath.Join(projectPath, "yarn.lock")) {
		return managerYarnBerry
	}
	return managerYarn
}

// isBerryLockFile reports whether a yarn.lock was written by Yarn Berry,
// whose lockfiles open with a __metadata key where Yarn classic's open
// with entries.
func isBerryLockFile(path string) bool {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return false
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return line == "__metadata:"
	}
	return false
}

// lockFileWarnings describes lockfiles that disagree with each other or
// with the declared package manager. chosen is the reported name.
func lockFileWarnings(declared declaration, present []lockFile, chosen string) []string {
	var managers, names []string
	for _, lock := range present {
		names = append(names, lock.name)
		if !slices.Contains(managers, lock.manager) {
			managers = append(managers, lock.manager)
		}
	}

	var warnings []string
	if len(managers) > 1 {
		warnings = append(warnings, fmt.Sprintf(
			"lockfiles from more than one package manager are present (%s); reporting %s",
			strings.Join(names, ", "), chosen))
	}
	if declared.manager != "" && len(managers) > 0 && !slices.Contains(managers, declared.manager) {
		warnings = append(warnings, fmt.Sprintf(
			"%s names %s but no %s lockfile is present (found %s); reporting %s",
			declared.source, declared.manager, declared.manager, strings.Join(names, ", "), chosen))
	}
	return warnings
}

// isFile reports whether path exists and is not a directory.
func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// applyPackageManager records the package manager, its declared version
// and its lockfile, reporting any warnings as workflow annotations.
// has_lock_file is recorded even when no lockfile is present.
func applyPackageManager(projectPath string, pkg *PackageJSON, languageSpecific map[string]interface{}) {
	manager, warnings := detectPackageManager(projectPath, pkg)
	for _, warning := range warnings {
		annotate("Node.js package manager: " + warning)
	}

	languageSpecific["package_manager"] = manager.name
	if manager.version != "" {
		languageSpecific["package_manager_version"] = manager.version
	}
	languageSpecific["has_lock_file"] = manager.lockFile != ""
	if manager.lockFile != "" {
		languageSpecific["lock_file"] = manager.lockFile
	}
}

// annotate reports a warning as a GitHub Actions workflow annotation.
func annotate(warning string) {
	fmt.Fprintf(os.Stderr, "::warning::%s\n", warning)
}
