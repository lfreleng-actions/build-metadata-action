// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package javascript

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// majorRange is an inclusive range of major versions. hi is anyMajor when
// the range has no upper bound.
type majorRange struct {
	lo, hi int
}

const anyMajor = math.MaxInt

// semver is a release version: major, minor and patch.
type semver [3]int

func compareSemver(a, b semver) int {
	for i := range a {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// bound is one end of an interval of versions. An upper bound may be
// unbounded; a lower bound is never, since 0.0.0 is the least version.
type bound struct {
	version   semver
	inclusive bool
	unbounded bool
}

// interval is the set of versions between two bounds.
type interval struct {
	lower, upper bound
}

var (
	everyVersion = interval{lower: bound{inclusive: true}, upper: bound{unbounded: true}}
	noVersion    = interval{lower: bound{inclusive: true}, upper: bound{}} // >=0.0.0 <0.0.0
)

func (i interval) empty() bool {
	if i.upper.unbounded {
		return false
	}
	c := compareSemver(i.lower.version, i.upper.version)
	return c > 0 || (c == 0 && (!i.lower.inclusive || !i.upper.inclusive))
}

// intersect narrows i to the versions other also admits.
func (i interval) intersect(other interval) interval {
	c := compareSemver(other.lower.version, i.lower.version)
	if c > 0 || (c == 0 && !other.lower.inclusive) {
		i.lower = other.lower
	}
	if !other.upper.unbounded {
		c = compareSemver(other.upper.version, i.upper.version)
		if i.upper.unbounded || c < 0 || (c == 0 && !other.upper.inclusive) {
			i.upper = other.upper
		}
	}
	return i
}

// majors projects a non-empty interval onto major versions. An exclusive
// upper bound of N.0.0 admits nothing in major N, prereleases aside.
func (i interval) majors() majorRange {
	r := majorRange{lo: i.lower.version[0], hi: anyMajor}
	if !i.upper.unbounded {
		r.hi = i.upper.version[0]
		if !i.upper.inclusive && i.upper.version[1] == 0 && i.upper.version[2] == 0 {
			r.hi--
		}
	}
	return r
}

// admittedMajors reports which major versions a version or npm semver
// range admits: those with at least one release satisfying it. It
// evaluates the range over full versions and ignores prerelease tags,
// returning one major range per non-empty "||" alternative. It reports
// false for text it cannot parse, and an empty or "*" range admits every
// major.
func admittedMajors(spec string) ([]majorRange, bool) {
	var union []majorRange
	for _, set := range strings.Split(spec, "||") {
		versions, ok := comparatorSetInterval(set)
		if !ok {
			return nil, false
		}
		if !versions.empty() {
			union = append(union, versions.majors())
		}
	}
	return union, true
}

// comparatorSetInterval intersects the comparators of one space-separated
// set, or reads a hyphen range ("1.2 - 3").
func comparatorSetInterval(set string) (interval, bool) {
	fields := strings.Fields(set)
	if len(fields) == 3 && fields[1] == "-" {
		return hyphenInterval(fields[0], fields[2])
	}

	result := everyVersion
	for i := 0; i < len(fields); i++ {
		comparator := fields[i]
		// npm allows whitespace between an operator and its version.
		if strings.Trim(comparator, "<>=^~") == "" && i+1 < len(fields) {
			i++
			comparator += fields[i]
		}
		versions, ok := comparatorInterval(comparator)
		if !ok {
			return interval{}, false
		}
		result = result.intersect(versions)
	}
	return result, true
}

// hyphenInterval reads "from - to": from fills missing parts with zeros,
// and a partial to admits everything it matches ("1 - 2" is <3.0.0).
func hyphenInterval(from, to string) (interval, bool) {
	lower, okLower := parsePartialVersion(from)
	upper, okUpper := parsePartialVersion(to)
	if !okLower || !okUpper {
		return interval{}, false
	}
	result := interval{lower: bound{version: floorVersion(lower), inclusive: true}}
	switch len(upper) {
	case 0:
		result.upper = bound{unbounded: true}
	case 3:
		result.upper = bound{version: floorVersion(upper), inclusive: true}
	default:
		result.upper = bound{version: nextVersion(upper)}
	}
	return result, true
}

// comparatorInterval returns the versions one comparator admits,
// following npm's desugaring of partial versions and x-ranges.
func comparatorInterval(comparator string) (interval, bool) {
	operand := strings.TrimLeft(comparator, "<>=^~")
	op := comparator[:len(comparator)-len(operand)]
	parts, ok := parsePartialVersion(operand)
	if !ok {
		return interval{}, false
	}
	floor := bound{version: floorVersion(parts), inclusive: true}

	if len(parts) == 0 {
		switch op {
		case "<", ">":
			return noVersion, true
		case "", "=", "<=", ">=", "^", "~", "~>":
			return everyVersion, true
		default:
			return interval{}, false
		}
	}

	exact := len(parts) == 3
	switch op {
	case "", "=":
		if exact {
			return interval{lower: floor, upper: floor}, true
		}
		return interval{lower: floor, upper: bound{version: nextVersion(parts)}}, true
	case ">=":
		return interval{lower: floor, upper: bound{unbounded: true}}, true
	case ">":
		if exact {
			return interval{lower: bound{version: floor.version}, upper: bound{unbounded: true}}, true
		}
		return interval{lower: bound{version: nextVersion(parts), inclusive: true}, upper: bound{unbounded: true}}, true
	case "<":
		return interval{lower: bound{inclusive: true}, upper: bound{version: floor.version}}, true
	case "<=":
		if exact {
			return interval{lower: bound{inclusive: true}, upper: floor}, true
		}
		return interval{lower: bound{inclusive: true}, upper: bound{version: nextVersion(parts)}}, true
	case "~", "~>":
		// ~1.2.3 and ~1.2 allow patch changes; ~1 allows minor changes.
		return interval{lower: floor, upper: bound{version: nextVersion(parts[:min(len(parts), 2)])}}, true
	case "^":
		// ^ allows changes below the first non-zero part it names.
		significant := len(parts)
		for i, part := range parts {
			if part != 0 {
				significant = i + 1
				break
			}
		}
		return interval{lower: floor, upper: bound{version: nextVersion(parts[:significant])}}, true
	default:
		return interval{}, false
	}
}

// floorVersion fills a partial version's missing parts with zeros.
func floorVersion(parts []int) semver {
	var version semver
	copy(version[:], parts)
	return version
}

// nextVersion returns the least version above every version a partial
// version matches: "1" gives 2.0.0, "1.2" gives 1.3.0, "1.2.3" 1.2.4.
func nextVersion(parts []int) semver {
	version := floorVersion(parts)
	version[len(parts)-1]++
	return version
}

// partialVersion is npm semver's strict x-range syntax: up to three parts,
// each a number without leading zeros or a wildcard, and a prerelease tag
// and build metadata only after all three.
var partialVersion = regexp.MustCompile(`^(\*|[xX]|0|[1-9][0-9]*)` +
	`(?:\.(\*|[xX]|0|[1-9][0-9]*)` +
	`(?:\.(\*|[xX]|0|[1-9][0-9]*)` +
	`(?:-(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*)?` +
	`(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?)?)?$`)

// parsePartialVersion parses a version that may be partial ("1", "1.2")
// or carry wildcards ("1.x", "*"), returning its numeric parts up to the
// first missing or wildcard part. Leading "v" and "=" characters, a
// prerelease tag and build metadata are dropped.
func parsePartialVersion(text string) ([]int, bool) {
	match := partialVersion.FindStringSubmatch(strings.TrimLeft(text, "v="))
	if match == nil {
		return nil, false
	}

	var numbers []int
	for _, part := range match[1:] {
		if part == "" || part == "x" || part == "X" || part == "*" {
			break
		}
		number, err := strconv.Atoi(part)
		if err != nil {
			return nil, false
		}
		numbers = append(numbers, number)
	}
	return numbers, true
}

// admitsMajor reports whether any of ranges contains major.
func admitsMajor(ranges []majorRange, major int) bool {
	for _, r := range ranges {
		if r.lo <= major && major <= r.hi {
			return true
		}
	}
	return false
}

// singleMajor returns the major version ranges admit when they admit
// exactly one.
func singleMajor(ranges []majorRange) (int, bool) {
	if len(ranges) == 0 {
		return 0, false
	}
	major := ranges[0].lo
	for _, r := range ranges {
		if r.lo != major || r.hi != major {
			return 0, false
		}
	}
	return major, true
}
