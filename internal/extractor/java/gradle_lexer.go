// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package java

// The brace bytes and the opening of an interpolation, spelled as
// escapes so that tools counting braces in this source stay balanced.
const (
	openBrace          = '\u007b'
	closeBrace         = '\u007d'
	interpolationStart = "$\u007b"
)

type gradleTokenKind int

const (
	gradleTokNewline gradleTokenKind = iota
	gradleTokComment
	gradleTokWord
	gradleTokString
	gradleTokLBrace
	gradleTokRBrace
	gradleTokPunct
)

type gradleToken struct {
	kind gradleTokenKind
	text string
	// spaceBefore records whitespace between this token and the previous
	// one on the same line.
	spaceBefore bool
	// plain marks a string in single or double quotes, closed on its own
	// line, holding no $ and no backslash. text is then its content.
	plain bool
	// quoted marks a Kotlin name in backticks, such as `version`. text
	// is then the name, which is never a keyword.
	quoted bool
	// last is the token's final byte in the source.
	last byte
}

// gradleLexer splits a build script into the tokens the version reader
// needs. It knows where comments and string literals end and nothing
// more of Groovy or Kotlin, besides Kotlin's names in backticks;
// failed is set when it cannot place the end of a block comment, a
// quoted string, a triple-quoted string, a Groovy slashy or
// dollar-slashy string, a Kotlin name in backticks, or an
// interpolation.
type gradleLexer struct {
	src    string
	pos    int
	kotlin bool
	failed bool
	toks   []gradleToken
	// space records white space since the last token on this line.
	space bool
}

// isGradleIdentByte reports whether c can be part of a name. Any
// non-ASCII byte counts, since both languages allow Unicode letters in
// names and other non-ASCII bytes appear only in strings and comments.
func isGradleIdentByte(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
		c >= 0x80
}

func (l *gradleLexer) hasPrefix(s string) bool {
	return len(l.src)-l.pos >= len(s) && l.src[l.pos:l.pos+len(s)] == s
}

func (l *gradleLexer) skipLineComment() {
	for l.pos < len(l.src) && l.src[l.pos] != '\n' {
		l.pos++
	}
}

// skipBlockComment consumes a comment starting at "/*". Kotlin block
// comments nest; Groovy ones do not.
func (l *gradleLexer) skipBlockComment() {
	l.pos += 2
	depth := 1
	for l.pos < len(l.src) {
		switch {
		case l.hasPrefix("*/"):
			l.pos += 2
			depth--
			if depth == 0 {
				return
			}
		case l.kotlin && l.hasPrefix("/*"):
			l.pos += 2
			depth++
		default:
			l.pos++
		}
	}
	l.failed = true
}

// skipQuoted consumes a string in single or double quotes, which must
// close on its own line; one that does not fails the lexer.
func (l *gradleLexer) skipQuoted() (content string, plain bool) {
	quote := l.src[l.pos]
	l.pos++
	start := l.pos
	plain = true
	interpolates := quote == '"'
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		switch c {
		case '\n':
			l.failed = true
			return "", false
		case quote:
			content = l.src[start:l.pos]
			l.pos++
			return content, plain
		case '\\':
			plain = false
			l.pos++
			if l.pos < len(l.src) && l.src[l.pos] != '\n' {
				l.pos++
			}
		case '$':
			plain = false
			l.pos++
			if interpolates && l.pos < len(l.src) && l.src[l.pos] == openBrace {
				l.pos++
				if !l.skipInterpolation(true) {
					l.failed = true
					return "", false
				}
			}
		default:
			l.pos++
		}
	}
	l.failed = true
	return "", false
}

// lexQuotedName consumes a Kotlin name in backticks, such as
// `version`, which must close on its own line and hold a name; one
// that does not fails the lexer.
func (l *gradleLexer) lexQuotedName() {
	l.pos++
	start := l.pos
	for l.pos < len(l.src) && l.src[l.pos] != '`' && l.src[l.pos] != '\n' {
		l.pos++
	}
	if l.pos >= len(l.src) || l.src[l.pos] != '`' || l.pos == start {
		l.failed = true
		return
	}
	name := l.src[start:l.pos]
	l.pos++
	l.emit(gradleToken{kind: gradleTokWord, text: name, quoted: true})
}

// skipTriple consumes a string in triple quotes, which may span lines.
func (l *gradleLexer) skipTriple() {
	delim := l.src[l.pos : l.pos+3]
	l.pos += 3
	interpolates := delim == `"""`
	// A Kotlin raw string has no escapes; a Groovy one does.
	escapes := !l.kotlin
	for l.pos < len(l.src) {
		switch {
		case l.hasPrefix(delim):
			l.pos += 3
			// Groovy and Kotlin both close on the last of a run of quotes.
			for l.pos < len(l.src) && l.src[l.pos] == delim[0] {
				l.pos++
			}
			return
		case escapes && l.src[l.pos] == '\\':
			l.pos = min(l.pos+2, len(l.src))
		case interpolates && l.hasPrefix(interpolationStart):
			l.pos += 2
			if !l.skipInterpolation(false) {
				l.failed = true
				return
			}
		default:
			l.pos++
		}
	}
	l.failed = true
}

// groovyKeywords are Groovy's reserved and contextual keywords. None
// can be the left side of a division, so a "/" after one opens a
// slashy string, as in assert /x/ or x in /y/. this, super, null, true
// and false are values and are left out, so a "/" after them divides.
var groovyKeywords = map[string]bool{
	"abstract": true, "as": true, "assert": true, "break": true,
	"case": true, "catch": true, "class": true, "const": true,
	"continue": true, "def": true, "default": true, "do": true,
	"else": true, "enum": true, "extends": true, "final": true,
	"finally": true, "for": true, "goto": true, "if": true,
	"implements": true, "import": true, "in": true, "instanceof": true,
	"interface": true, "native": true, "new": true, "package": true,
	"permits": true, "private": true, "protected": true, "public": true,
	"record": true, "return": true, "sealed": true, "static": true,
	"strictfp": true, "switch": true, "synchronized": true,
	"threadsafe": true, "throw": true, "throws": true, "trait": true,
	"transient": true, "try": true, "var": true, "while": true,
	"yield": true,
}

// opensSlashy reports whether a "/" at the current position, in a
// Groovy script, starts a slashy string rather than dividing. It does
// where a division cannot stand: when the last token before it,
// skipping white space and comments, is not a name, a number, ")",
// "]", "}", a closing quote, "++" or "--". A Groovy keyword is not a
// name. The token before the "/" decides alone, so a slashy string at
// the start of a statement, or after the ")" of an if, for or while
// header, is read as a division.
func (l *gradleLexer) opensSlashy() bool {
	for k := len(l.toks) - 1; k >= 0; k-- {
		prev := l.toks[k]
		if prev.kind == gradleTokNewline || prev.kind == gradleTokComment {
			continue
		}
		if prev.kind == gradleTokWord {
			// A word after "." names a property, as in clazz.class / 2,
			// with white space, comments or line breaks between them or not.
			if afterDot(l.toks, k) {
				return false
			}
			return groovyKeywords[prev.text]
		}
		if (isGradlePunct(prev, "+") || isGradlePunct(prev, "-")) && !prev.spaceBefore &&
			k > 0 && isGradlePunct(l.toks[k-1], prev.text) {
			return false
		}
		c := prev.last
		return !isGradleIdentByte(c) && c != ')' && c != ']' && c != closeBrace && c != '"' && c != '\''
	}
	return true
}

// afterDot reports whether the last token before k, past line breaks
// and comments, is ".".
func afterDot(toks []gradleToken, k int) bool {
	for k--; k >= 0; k-- {
		if toks[k].kind != gradleTokNewline && toks[k].kind != gradleTokComment {
			return isGradlePunct(toks[k], ".")
		}
	}
	return false
}

// skipSlashy consumes a Groovy slashy string, /.../, which may span
// lines and in which "\/" escapes a slash.
func (l *gradleLexer) skipSlashy() {
	l.pos++
	for l.pos < len(l.src) {
		switch {
		case l.hasPrefix(`\/`):
			l.pos += 2
		case l.hasPrefix(interpolationStart):
			l.pos += 2
			if !l.skipInterpolation(false) {
				l.failed = true
				return
			}
		case l.src[l.pos] == '/':
			l.pos++
			return
		default:
			l.pos++
		}
	}
	l.failed = true
}

// skipDollarSlashy consumes a Groovy dollar-slashy string, $/.../$,
// which may span lines and in which "$$" and "$/" are escapes.
func (l *gradleLexer) skipDollarSlashy() {
	l.pos += 2
	for l.pos < len(l.src) {
		switch {
		case l.hasPrefix("/$"):
			l.pos += 2
			return
		case l.hasPrefix("$$"), l.hasPrefix("$/"):
			l.pos += 2
		case l.hasPrefix(interpolationStart):
			l.pos += 2
			if !l.skipInterpolation(false) {
				l.failed = true
				return
			}
		default:
			l.pos++
		}
	}
	l.failed = true
}

// skipInterpolation consumes the code of a ${ ... } after its opening
// brace. The code is lexed as the script is, so its comments, strings,
// slashy strings included, and braces are placed by the same rules,
// and its tokens are then dropped. In a string that must close on its
// line, the end of the line ends the interpolation too and it reports
// false; otherwise it reports false only at the end of the script.
func (l *gradleLexer) skipInterpolation(singleLine bool) bool {
	mark, space := len(l.toks), l.space
	defer func() { l.toks, l.space = l.toks[:mark], space }()
	// The opening brace stands before the code, so a "/" that starts
	// it opens a slashy string, as it does in Groovy.
	l.space = false
	l.emit(gradleToken{kind: gradleTokLBrace})
	depth := 1
	for l.pos < len(l.src) && !l.failed {
		switch l.src[l.pos] {
		case '\n':
			if singleLine {
				return false
			}
		case openBrace:
			depth++
		case closeBrace:
			depth--
			if depth == 0 {
				l.pos++
				return true
			}
		}
		l.lexToken()
	}
	return false
}

func (l *gradleLexer) emit(tok gradleToken) {
	tok.spaceBefore = l.space
	if l.pos > 0 {
		tok.last = l.src[l.pos-1]
	}
	l.toks = append(l.toks, tok)
	l.space = false
}

// lexToken consumes the white space or the token at the current
// position.
func (l *gradleLexer) lexToken() {
	c := l.src[l.pos]
	switch {
	case c == '\n':
		l.pos++
		l.emit(gradleToken{kind: gradleTokNewline})
	case c == ' ' || c == '\t' || c == '\r' || c == '\f':
		l.pos++
		l.space = true
	case l.hasPrefix("//"):
		l.skipLineComment()
		l.emit(gradleToken{kind: gradleTokComment})
	case l.hasPrefix("/*"):
		l.skipBlockComment()
		l.emit(gradleToken{kind: gradleTokComment})
	case l.hasPrefix(`"""`), l.hasPrefix("'''"):
		l.skipTriple()
		l.emit(gradleToken{kind: gradleTokString})
	case c == '"' || c == '\'':
		content, plain := l.skipQuoted()
		l.emit(gradleToken{kind: gradleTokString, text: content, plain: plain})
	case !l.kotlin && l.hasPrefix("$/"):
		l.skipDollarSlashy()
		l.emit(gradleToken{kind: gradleTokString})
	case !l.kotlin && c == '/' && l.opensSlashy():
		l.skipSlashy()
		l.emit(gradleToken{kind: gradleTokString})
	case l.kotlin && c == '`':
		l.lexQuotedName()
	case c == openBrace:
		l.pos++
		l.emit(gradleToken{kind: gradleTokLBrace})
	case c == closeBrace:
		l.pos++
		l.emit(gradleToken{kind: gradleTokRBrace})
	case isGradleIdentByte(c):
		start := l.pos
		for l.pos < len(l.src) && isGradleIdentByte(l.src[l.pos]) {
			l.pos++
		}
		l.emit(gradleToken{kind: gradleTokWord, text: l.src[start:l.pos]})
	default:
		l.pos++
		l.emit(gradleToken{kind: gradleTokPunct, text: string(c)})
	}
}

func lexGradleScript(src string, kotlin bool) ([]gradleToken, bool) {
	l := &gradleLexer{src: src, kotlin: kotlin}
	for l.pos < len(l.src) && !l.failed {
		l.lexToken()
	}
	if l.failed {
		return nil, false
	}
	// Comments are never tokens to the reader: drop them, keeping the
	// white space they stand for.
	toks := l.toks[:0]
	afterComment := false
	for _, tok := range l.toks {
		if tok.kind == gradleTokComment {
			afterComment = true
			continue
		}
		if afterComment && tok.kind != gradleTokNewline {
			tok.spaceBefore = true
		}
		afterComment = false
		toks = append(toks, tok)
	}
	return toks, true
}

func isGradlePunct(tok gradleToken, text string) bool {
	return tok.kind == gradleTokPunct && tok.text == text
}

// isGradleKeyword reports whether tok is the keyword text: a word that
// is not a Kotlin name in backticks, as `if` is a name and not a
// keyword.
func isGradleKeyword(tok gradleToken, text string) bool {
	return tok.kind == gradleTokWord && !tok.quoted && tok.text == text
}
