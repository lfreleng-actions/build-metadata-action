// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package rust

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var errMemberGlob = errors.New("invalid workspace members glob")

// expandMemberGlob expands a workspace members entry the way cargo does
// through the glob crate, which differs from filepath.Glob: "**" as a
// whole component matches zero or more directories (a trailing "**"
// matches every descendant but not the directory itself), "[!a]"
// negates a class while "^" is literal, a leading "]" in a class is
// literal, and a backslash escapes nothing. Like cargo, it keeps
// directories alone. base is the directory a relative entry starts
// from; an absolute entry starts at the root of its own volume.
// Verified against cargo 1.99.
//
// The manifest comes from the repository, so the walk lists no
// directory whose real path lies outside limit: an entry such as "/**"
// or "../*" cannot enumerate the runner. The walk visits each
// (directory, component) pair once, a directory counting once across
// its symlinked spellings after the last ".." component, so neither
// repeated "**" components nor symlink aliases can make it revisit a
// tree combinatorially. It reads the filesystem through cache, which
// lists each real directory once across a workspace's members entries.
func expandMemberGlob(base, pattern string, cache *dirCache) ([]string, error) {
	volume := filepath.VolumeName(pattern)
	var components []string
	for _, component := range strings.FieldsFunc(pattern[len(volume):], isGlobSeparator) {
		if component != "**" && strings.Contains(component, "**") {
			return nil, errMemberGlob
		}
		components = append(components, component)
	}
	matchers := make([]*regexp.Regexp, len(components))
	for i, component := range components {
		if component == "**" || !strings.ContainsAny(component, "*?[") {
			continue
		}
		matcher, err := componentMatcher(component)
		if err != nil {
			return nil, err
		}
		matchers[i] = matcher
	}

	// Cargo applies ".." lexically, so until the last ".." the spelling
	// of a directory decides where the walk goes.
	realFrom := 0
	for i, component := range components {
		if component == ".." {
			realFrom = i + 1
		}
	}

	w := memberGlobWalk{
		components: components,
		matchers:   matchers,
		realFrom:   realFrom,
		cache:      cache,
		found:      map[string]bool{},
		visited:    map[memberGlobState]bool{},
	}
	if filepath.IsAbs(pattern) {
		base = volume + string(filepath.Separator)
	}
	w.walk(base, 0)

	matches := make([]string, 0, len(w.found))
	for match := range w.found {
		matches = append(matches, match)
	}
	sort.Strings(matches)
	return matches, nil
}

// memberGlobState is a directory reached with the pattern consumed up
// to component i. Past the last ".." component the directory is a real
// path, so every symlinked spelling of it shares one walk: the
// directories it matches no longer depend on the spelling.
type memberGlobState struct {
	dir string
	i   int
}

// memberGlobWalk holds one expansion; its memo table keeps the walk
// linear in the number of (directory, component) pairs.
type memberGlobWalk struct {
	components []string
	matchers   []*regexp.Regexp
	realFrom   int // first component with no ".." at or after it
	cache      *dirCache
	found      map[string]bool
	visited    map[memberGlobState]bool
}

func (w *memberGlobWalk) walk(dir string, i int) {
	key := dir
	if i >= w.realFrom {
		if resolved := w.cache.resolve(dir); resolved != "" {
			key = resolved
		}
	}
	state := memberGlobState{key, i}
	if w.visited[state] {
		return
	}
	w.visited[state] = true
	if i == len(w.components) {
		if w.cache.isDir(dir) {
			w.found[dir] = true
		}
		return
	}
	switch {
	case w.components[i] == "**":
		// "**" either stops here or consumes one subdirectory and
		// stays. A trailing "**" never matches dir itself. Symbolic
		// links are not followed, so a link cycle cannot make it
		// endless.
		if i+1 < len(w.components) {
			w.walk(dir, i+1)
		}
		for _, entry := range w.cache.list(dir) {
			if entry.IsDir() {
				sub := filepath.Join(dir, entry.Name())
				w.walk(sub, i+1)
				w.walk(sub, i)
			}
		}
	case w.matchers[i] != nil:
		for _, entry := range w.cache.list(dir) {
			if w.matchers[i].MatchString(entry.Name()) {
				w.walk(filepath.Join(dir, entry.Name()), i+1)
			}
		}
	default:
		w.walk(filepath.Join(dir, w.components[i]), i+1)
	}
}

// dirCache reads directories for glob expansion, each real directory
// once, however many symlinked spellings reach it. It lists no
// directory whose real path lies outside limit.
type dirCache struct {
	limit    string
	real     map[string]string
	listings map[string][]fs.DirEntry
	dirs     map[string]bool
}

func newDirCache(limit string) *dirCache {
	return &dirCache{
		limit:    limit,
		real:     map[string]string{},
		listings: map[string][]fs.DirEntry{},
		dirs:     map[string]bool{},
	}
}

// resolve returns the real absolute path of dir, or "" when it has none.
func (c *dirCache) resolve(dir string) string {
	resolved, ok := c.real[dir]
	if !ok {
		var err error
		resolved, err = filepath.EvalSymlinks(dir)
		if err == nil {
			resolved, err = filepath.Abs(resolved)
		}
		if err != nil {
			resolved = ""
		}
		c.real[dir] = resolved
	}
	return resolved
}

// list returns the entries of dir. A directory outside the limit, or
// one that cannot be read, contributes no entries.
func (c *dirCache) list(dir string) []fs.DirEntry {
	resolved := c.resolve(dir)
	if resolved == "" || !isWithin(c.limit, resolved) {
		return nil
	}
	entries, ok := c.listings[resolved]
	if !ok {
		entries = c.read(resolved)
		c.listings[resolved] = entries
	}
	return entries
}

func (c *dirCache) read(dir string) []fs.DirEntry {
	entries, err := listDir(dir)
	if err != nil {
		return nil
	}
	return entries
}

// isDir reports whether dir exists as a directory.
func (c *dirCache) isDir(dir string) bool {
	known, ok := c.dirs[dir]
	if !ok {
		info, err := os.Stat(dir)
		known = err == nil && info.IsDir()
		c.dirs[dir] = known
	}
	return known
}

// listDir reads a directory; tests replace it to watch the walk.
var listDir = os.ReadDir

// isGlobSeparator splits a pattern as the glob crate does: on "/", and
// on Windows on "\" as well.
func isGlobSeparator(r rune) bool {
	return r == '/' || r == filepath.Separator
}

// componentMatcher translates one glob component into an anchored
// regular expression.
func componentMatcher(component string) (*regexp.Regexp, error) {
	var expr strings.Builder
	expr.WriteString("(?s)^")
	runes := []rune(component)
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case '*':
			expr.WriteString(".*")
		case '?':
			expr.WriteString(".")
		case '[':
			class, next, err := globClass(runes, i+1)
			if err != nil {
				return nil, err
			}
			expr.WriteString(class)
			i = next
		default:
			expr.WriteString(regexp.QuoteMeta(string(runes[i])))
		}
	}
	expr.WriteString("$")
	return regexp.Compile(expr.String())
}

// globClass translates the class starting after "[" at start and returns
// it with the index of its closing "]".
func globClass(runes []rune, start int) (string, int, error) {
	var class strings.Builder
	class.WriteString("[")
	i := start
	if i < len(runes) && runes[i] == '!' {
		class.WriteString("^")
		i++
	}
	first := true
	for ; i < len(runes); i++ {
		if runes[i] == ']' && !first {
			class.WriteString("]")
			return class.String(), i, nil
		}
		first = false
		class.WriteString(classLiteral(runes[i]))
		if i+2 < len(runes) && runes[i+1] == '-' && runes[i+2] != ']' {
			class.WriteString("-" + classLiteral(runes[i+2]))
			i += 2
		}
	}
	return "", 0, errMemberGlob
}

func classLiteral(r rune) string {
	if strings.ContainsRune(`\]-^[`, r) {
		return `\` + string(r)
	}
	return string(r)
}
