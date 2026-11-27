// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package java

// scriptVersionKind says what a Gradle build script declares about the
// project version.
type scriptVersionKind int

const (
	// scriptVersionNone: no statement sets the project version.
	scriptVersionNone scriptVersionKind = iota
	// scriptVersionLiteral: the deciding statement assigns one plain
	// string literal.
	scriptVersionLiteral
	// scriptVersionComputed: the deciding statement assigns anything
	// else, or the script cannot be read with confidence.
	scriptVersionComputed
)

// readGradleScriptVersion reads the project version a Gradle build
// script declares. A statement counts when it starts a line outside
// comments, strings, parentheses and brackets, sits at the top level
// or directly inside a top-level allprojects block, and is version =
// <value>, project.version = <value>, or in Groovy alone version
// <value> or version(<value>), or a compound assignment such as
// version += <value>. The last such statement decides. A compound
// assignment is always computed. Otherwise its value is a literal only
// when the rest of its line is one plain string in single or double
// quotes, followed by at most a semicolon and a comment; anything else
// is computed, as is a statement that is the unbraced body of an if,
// for, while, else or do on an earlier line, or sits directly inside
// an allprojects block that is one; the while that ends a do loop
// heads no statement. A line after one that ends in "." continues
// that statement and starts none. Comments count as white space
// throughout, and a Kotlin name in backticks is that name but never a
// keyword. A script that declares a local named version or project
// where statements count (see declaresGradleLocal) computes its
// version, wherever the declaration stands, and so does one that
// assigns version or project.version, plainly or by a compound
// assignment, where statements count but not first on its line, as in
// group = 'a'; version = '2.0.0'; in Groovy, so does a bare version
// there directly followed by a string literal or "(", as in
// allprojects { version('2.0.0') }. Where the script cannot
// be read with confidence (an unterminated comment, quoted,
// triple-quoted, slashy or dollar-slashy string, Kotlin name in
// backticks, or braces, parentheses or brackets that do not balance)
// the whole script counts as computing its version: the reader
// declines rather than guess.
func readGradleScriptVersion(src string, kotlin bool) (string, scriptVersionKind) {
	toks, ok := lexGradleScript(src, kotlin)
	if !ok {
		return "", scriptVersionComputed
	}

	kind := scriptVersionNone
	value := ""
	n := gradleNesting{
		toks:        toks,
		prev:        make([]int, len(toks)),
		controlEnds: make([]bool, len(toks)),
		doEnds:      make([]bool, len(toks)),
	}
	lineStart, last := 0, -1
	for i, tok := range toks {
		// Record the last token before i that is not a line break as the
		// scan goes, so that every look back costs constant time.
		n.prev[i] = last
		if tok.kind != gradleTokNewline {
			last = i
		}
		if nested, ok := n.step(i, lineStart); !ok {
			return "", scriptVersionComputed
		} else if nested {
			continue
		}
		if declaresGradleLocal(&n, i) {
			return "", scriptVersionComputed
		}
		if tok.kind == gradleTokNewline {
			// A line after a trailing "." continues the statement, as in
			// project(":child").\n    version = "8.8.8", so it starts none.
			if p := n.prev[i]; p < 0 || !isGradlePunct(toks[p], ".") {
				lineStart = i + 1
			}
			continue
		}
		if !n.atTopLevel() {
			continue
		}
		if i != lineStart {
			// A setter the reader does not count, as in
			// group = 'a'; version = '2.0.0' or
			// allprojects { version('2.0.0') }, is a reason to decline.
			if assignsGradleVersion(&n, i) || callsGradleVersion(&n, i, kotlin) {
				return "", scriptVersionComputed
			}
			continue
		}
		if v, k, found := matchGradleVersionStatement(toks, i, kotlin); found {
			value, kind = v, k
			if n.governedByControl(i) || (len(n.blocks) == 1 && n.blocks[0].conditional) {
				value, kind = "", scriptVersionComputed
			}
		}
	}
	if len(n.blocks) != 0 || len(n.groups) != 0 {
		return "", scriptVersionComputed
	}
	return value, kind
}

// gradleBlock is an open brace: whether it opened a top-level
// allprojects block, whether that block is the unbraced body of a
// control statement, and whether it opened the body of a do. For an
// allprojects block, paramsEnd is the index of the arrow "->" that
// ends its closure parameters, or -1 when it has none.
type gradleBlock struct {
	allprojects bool
	conditional bool
	doBody      bool
	paramsEnd   int
}

// gradleGroup is an open parenthesis or bracket, whether it opened the
// condition of an if, for or while, and whether it directly follows
// def, val or var, opening the names of a destructuring declaration.
type gradleGroup struct {
	open        string
	control     bool
	destructure bool
}

// gradleNesting tracks the braces, parentheses and brackets open at
// each token. prev holds, for each token, the index of the last token
// before it that is not a line break, or -1. controlEnds marks the
// tokens that closed the condition of an if, for or while; doEnds marks
// the braces that closed a do body.
type gradleNesting struct {
	toks        []gradleToken
	prev        []int
	blocks      []gradleBlock
	groups      []gradleGroup
	controlEnds []bool
	doEnds      []bool
}

// step takes the token at i into account. It reports whether the token
// is a brace, parenthesis or bracket, and ok false when one closes
// what is not open.
func (n *gradleNesting) step(i, lineStart int) (nested, ok bool) {
	tok := n.toks[i]
	switch {
	case isGradlePunct(tok, "("), isGradlePunct(tok, "["):
		control := tok.text == "(" && n.isControlKeyword(i)
		destructure := tok.text == "(" &&
			(n.previousKeywordIs(i, "def") || n.previousKeywordIs(i, "val") || n.previousKeywordIs(i, "var"))
		n.groups = append(n.groups, gradleGroup{tok.text, control, destructure})
	case isGradlePunct(tok, ")"), isGradlePunct(tok, "]"):
		want := "("
		if tok.text == "]" {
			want = "["
		}
		if len(n.groups) == 0 || n.groups[len(n.groups)-1].open != want {
			return true, false
		}
		n.controlEnds[i] = n.groups[len(n.groups)-1].control
		n.groups = n.groups[:len(n.groups)-1]
	case tok.kind == gradleTokLBrace:
		allprojects := len(n.blocks) == 0 && len(n.groups) == 0 && i == lineStart+1 &&
			n.toks[lineStart].kind == gradleTokWord && n.toks[lineStart].text == "allprojects"
		paramsEnd := -1
		if allprojects {
			paramsEnd = gradleClosureParamsEnd(n.toks, i)
		}
		n.blocks = append(n.blocks, gradleBlock{
			allprojects: allprojects,
			conditional: allprojects && n.governedByControl(lineStart),
			doBody:      n.previousKeywordIs(i, "do"),
			paramsEnd:   paramsEnd,
		})
	case tok.kind == gradleTokRBrace:
		if len(n.blocks) == 0 {
			return true, false
		}
		n.doEnds[i] = n.blocks[len(n.blocks)-1].doBody
		n.blocks = n.blocks[:len(n.blocks)-1]
	default:
		return false, true
	}
	return true, true
}

// gradleClosureParamsEnd returns the index of the arrow "->" that ends
// the parameters of the closure whose "{" is at b, or -1 when it has
// none. The parameters run from the brace to the first arrow outside
// brackets and nested braces, across line breaks, so a closure-valued
// default such as version = {} -> is skipped whole. They end without
// one at the block's own closing brace or a ";" outside brackets. Top-
// level allprojects blocks never nest, and each scans only up to its
// own closing brace, so the scans together stay linear.
func gradleClosureParamsEnd(toks []gradleToken, b int) int {
	depth, braces := 0, 0
	for k := b + 1; k < len(toks); k++ {
		tok := toks[k]
		switch {
		case tok.kind == gradleTokLBrace:
			braces++
		case tok.kind == gradleTokRBrace:
			if braces == 0 {
				return -1
			}
			braces--
		case braces != 0:
		case isGradlePunct(tok, "("), isGradlePunct(tok, "["):
			depth++
		case isGradlePunct(tok, ")"), isGradlePunct(tok, "]"):
			depth--
		case depth != 0:
		case isGradlePunct(tok, ";"):
			return -1
		case isGradlePunct(tok, "-") && k+1 < len(toks) &&
			isGradlePunct(toks[k+1], ">") && !toks[k+1].spaceBefore:
			return k
		}
	}
	return -1
}

// atTopLevel reports whether a statement here sits at the top level of
// the script or directly inside a top-level allprojects block.
func (n *gradleNesting) atTopLevel() bool {
	return len(n.groups) == 0 && (len(n.blocks) == 0 || (len(n.blocks) == 1 && n.blocks[0].allprojects))
}

// isControlKeyword reports whether the last token before i, past line
// breaks, is if, for or while, so that the "(" at i opens its condition.
// A while after the closing brace of a do body ends a do-while loop and
// heads no statement.
func (n *gradleNesting) isControlKeyword(i int) bool {
	k := n.prev[i]
	if k < 0 || n.toks[k].kind != gradleTokWord || n.toks[k].quoted {
		return false
	}
	switch n.toks[k].text {
	case "if", "for":
		return true
	case "while":
		before := n.prev[k]
		return before < 0 || !n.doEnds[before]
	}
	return false
}

// previousKeywordIs reports whether the last token before i, past line
// breaks, is the keyword text.
func (n *gradleNesting) previousKeywordIs(i int, text string) bool {
	k := n.prev[i]
	return k >= 0 && isGradleKeyword(n.toks[k], text)
}

// governedByControl reports whether the statement starting at token i
// is the unbraced body of a control statement: whether the last token
// before it, past line breaks, closes the condition of an if, for or
// while, or is else or do.
func (n *gradleNesting) governedByControl(i int) bool {
	k := n.prev[i]
	switch {
	case k < 0:
		return false
	case n.toks[k].kind == gradleTokWord:
		return isGradleKeyword(n.toks[k], "else") || isGradleKeyword(n.toks[k], "do")
	}
	return n.controlEnds[k]
}

// gradleDeclarationWords are the keywords that, written directly before
// a name, declare a local by that name: def, var, val, final and Java's
// primitive types.
var gradleDeclarationWords = map[string]bool{
	"def": true, "var": true, "val": true, "final": true,
	"boolean": true, "byte": true, "char": true, "short": true,
	"int": true, "long": true, "float": true, "double": true,
}

// declaresGradleLocal reports whether the token at i declares a local
// named version or project, judged from its neighbouring tokens alone.
// The name must be a word, not after a ".", where statements count:
// at the top level or directly inside a top-level allprojects block.
// It declares a local when the token before it, past comments and on
// the same line or as the last token of the line above, is one of
// gradleDeclarationWords; when, on the same line and outside
// brackets, it is the end of a type: a capitalised name, a "]", or a
// ">" that is not the arrow "->"; when, outside brackets, it is a ","
// and no ":" follows the name, which would make it a named argument;
// or when the name sits inside the brackets that directly follow def,
// val or var, as in def (version, project) = …. A version directly
// before the arrow "->", or outside brackets among the parameters of a
// top-level allprojects closure, as in allprojects { version = null ->,
// names a closure parameter and declares one too; project there names
// the project, as in allprojects { project ->.
func declaresGradleLocal(n *gradleNesting, i int) bool {
	tok := n.toks[i]
	if tok.kind != gradleTokWord || (tok.text != "version" && tok.text != "project") {
		return false
	}
	if len(n.blocks) > 1 || (len(n.blocks) == 1 && !n.blocks[0].allprojects) {
		return false
	}
	p := n.prev[i]
	if p >= 0 && isGradlePunct(n.toks[p], ".") {
		return false
	}
	if len(n.groups) != 0 && n.groups[len(n.groups)-1].destructure {
		return true
	}
	if tok.text == "version" && i+2 < len(n.toks) && isGradlePunct(n.toks[i+1], "-") &&
		isGradlePunct(n.toks[i+2], ">") && !n.toks[i+2].spaceBefore {
		return true
	}
	if tok.text == "version" && len(n.blocks) == 1 && len(n.groups) == 0 && i < n.blocks[0].paramsEnd {
		return true
	}
	if p < 0 {
		return false
	}
	before := n.toks[p]
	if before.kind == gradleTokWord && !before.quoted && gradleDeclarationWords[before.text] {
		return true
	}
	if len(n.groups) != 0 {
		return false
	}
	if p == i-1 && endsGradleType(n.toks, p) {
		return true
	}
	return isGradlePunct(before, ",") && (i+1 >= len(n.toks) || !isGradlePunct(n.toks[i+1], ":"))
}

// endsGradleType reports whether the token at p can end a type: a
// capitalised name, a "]", or a ">" that is not the arrow "->".
func endsGradleType(toks []gradleToken, p int) bool {
	tok := toks[p]
	switch {
	case tok.kind == gradleTokWord:
		return tok.text != "" && tok.text[0] >= 'A' && tok.text[0] <= 'Z'
	case isGradlePunct(tok, "]"):
		return true
	case isGradlePunct(tok, ">"):
		return tok.spaceBefore || p == 0 || !isGradlePunct(toks[p-1], "-")
	}
	return false
}

// matchGradleVersionStatement reports whether the tokens at i, which
// start a line, set the project version, and what they set it to.
func matchGradleVersionStatement(toks []gradleToken, i int, kotlin bool) (string, scriptVersionKind, bool) {
	at := func(j int) gradleToken {
		if j < len(toks) {
			return toks[j]
		}
		return gradleToken{kind: gradleTokNewline}
	}
	isWord := func(j int, text string) bool {
		return at(j).kind == gradleTokWord && at(j).text == text
	}

	bare := isWord(i, "version")
	j := i + 1
	if !bare {
		if !isWord(i, "project") || !isGradlePunct(at(i+1), ".") || !isWord(i+2, "version") {
			return "", scriptVersionNone, false
		}
		j = i + 3
	}

	next := at(j)
	switch {
	case isCompoundAssignment(toks, j):
		return "", scriptVersionComputed, true
	case isGradlePunct(next, "="):
		// Rule out ==, ===, =~ and ==~; any other value, such as
		// ("2.0.0"), is assigned.
		if after := at(j + 1); !after.spaceBefore && (isGradlePunct(after, "=") || isGradlePunct(after, "~")) {
			return "", scriptVersionNone, false
		}
		v, k := classifyGradleValue(toks, j+1, false)
		return v, k, true
	case kotlin || !bare:
		return "", scriptVersionNone, false
	case isGradlePunct(next, "("):
		v, k := classifyGradleValue(toks, j+1, true)
		return v, k, true
	case next.spaceBefore && (next.kind == gradleTokString || next.kind == gradleTokWord):
		v, k := classifyGradleValue(toks, j, false)
		return v, k, true
	}
	return "", scriptVersionNone, false
}

// assignsGradleVersion reports whether the tokens at i assign the
// project version: version or project.version, not after a ".",
// followed by a compound assignment or by an "=" that does not begin
// ==, === or =~.
func assignsGradleVersion(n *gradleNesting, i int) bool {
	toks := n.toks
	isWord := func(j int, text string) bool {
		return j < len(toks) && toks[j].kind == gradleTokWord && toks[j].text == text
	}
	if p := n.prev[i]; p >= 0 && isGradlePunct(toks[p], ".") {
		return false
	}
	j := i + 1
	switch {
	case isWord(i, "version"):
	case isWord(i, "project") && j < len(toks) && isGradlePunct(toks[j], ".") && isWord(i+2, "version"):
		j = i + 3
	default:
		return false
	}
	if isCompoundAssignment(toks, j) {
		return true
	}
	if j >= len(toks) || !isGradlePunct(toks[j], "=") {
		return false
	}
	after := j + 1
	return after >= len(toks) || toks[after].spaceBefore ||
		(!isGradlePunct(toks[after], "=") && !isGradlePunct(toks[after], "~"))
}

// callsGradleVersion reports whether the tokens at i, in a Groovy
// script, call the version setter in either call form: a bare version,
// not after a ".", directly followed by a string literal or by "(", as
// in version '2.0.0' or version('2.0.0'). A version followed by a
// word, as in version as String, reads it.
func callsGradleVersion(n *gradleNesting, i int, kotlin bool) bool {
	toks := n.toks
	if kotlin || toks[i].kind != gradleTokWord || toks[i].text != "version" || i+1 >= len(toks) {
		return false
	}
	if p := n.prev[i]; p >= 0 && isGradlePunct(toks[p], ".") {
		return false
	}
	next := toks[i+1]
	return next.kind == gradleTokString || isGradlePunct(next, "(")
}

// gradleCompoundOperators are the operators that, written directly
// before "=", form a compound assignment such as += or ?=.
var gradleCompoundOperators = []string{"+", "-", "*", "/", "%", "&", "|", "^", "?", "**", "<<", ">>", ">>>"}

// isCompoundAssignment reports whether the tokens at j, with no space
// between them, are a compound assignment operator: one of
// gradleCompoundOperators followed by "=". Comparisons such as >=, <=
// and != are not.
func isCompoundAssignment(toks []gradleToken, j int) bool {
	for _, op := range gradleCompoundOperators {
		k := j
		for n := 0; n < len(op); n++ {
			if k >= len(toks) || !isGradlePunct(toks[k], op[n:n+1]) || (n > 0 && toks[k].spaceBefore) {
				break
			}
			k++
		}
		if k == j+len(op) && k < len(toks) && isGradlePunct(toks[k], "=") && !toks[k].spaceBefore {
			return true
		}
	}
	return false
}

// classifyGradleValue reads the value starting at token j. It is a
// literal only when it is one plain string (closed by ")" for the call
// form), followed by at most a semicolon and comments up to the end of
// the line, and the next line does not continue the expression.
func classifyGradleValue(toks []gradleToken, j int, call bool) (string, scriptVersionKind) {
	at := func(k int) gradleToken {
		if k < len(toks) {
			return toks[k]
		}
		return gradleToken{kind: gradleTokNewline}
	}
	str := at(j)
	if str.kind != gradleTokString || !str.plain {
		return "", scriptVersionComputed
	}
	j++
	if call {
		if !isGradlePunct(at(j), ")") {
			return "", scriptVersionComputed
		}
		j++
	}
	if isGradlePunct(at(j), ";") {
		j++
	}
	if at(j).kind != gradleTokNewline {
		return "", scriptVersionComputed
	}
	// A line that opens with "." or "?." continues the expression in
	// both languages, as in version = "1.0"\n    .trim().
	for j < len(toks) && toks[j].kind == gradleTokNewline {
		j++
	}
	if j < len(toks) && (isGradlePunct(toks[j], ".") || isGradlePunct(toks[j], "?")) {
		return "", scriptVersionComputed
	}
	return str.text, scriptVersionLiteral
}
