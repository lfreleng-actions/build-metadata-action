// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package python

import "strings"

// The braces that open and close an f-string's replacement field.
const openBrace, closeBrace = '{', '}'

// pyTokenKind classifies a token of Python source.
type pyTokenKind int

const (
	pyName pyTokenKind = iota
	pyString
	pyNewline
	pyOther
)

// pyToken is one token of Python source. For a string, value holds the
// text between the quotes and plain reports a literal with no prefix
// other than u or r, so neither an f-string nor a bytes literal.
// unsure marks a string whose end the lexer could not place with
// certainty, such as one left unterminated.
type pyToken struct {
	kind   pyTokenKind
	text   string
	value  string
	plain  bool
	unsure bool
}

func isPyNameByte(c byte, first bool) bool {
	switch {
	case c == '_', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= 0x80:
		return true
	case c >= '0' && c <= '9':
		return !first
	}
	return false
}

// lexPython splits Python source into names, string literals, logical
// line ends and other tokens, dropping whitespace and comments. A line
// end inside brackets or after a backslash continues the line, as in
// Python, so it yields no token. It never evaluates the source; it only
// finds where names, strings and punctuation lie.
func lexPython(src string) []pyToken {
	var tokens []pyToken
	depth := 0
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == ' ', c == '\t', c == '\r', c == '\f':
			i++
		case c == '\\':
			i++
			if strings.HasPrefix(src[i:], "\r\n") {
				i += 2
			} else if i < len(src) && src[i] == '\n' {
				i++
			}
		case c == '\n':
			if depth == 0 && len(tokens) > 0 && tokens[len(tokens)-1].kind != pyNewline {
				tokens = append(tokens, pyToken{kind: pyNewline, text: "\n"})
			}
			i++
		case c == '#':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case c == '"', c == '\'':
			var tok pyToken
			tok, i = lexPyString(src, i, "")
			tokens = append(tokens, tok)
		case isPyNameByte(c, true):
			start := i
			for i < len(src) && isPyNameByte(src[i], false) {
				i++
			}
			name := src[start:i]
			if i < len(src) && (src[i] == '"' || src[i] == '\'') && isPyStringPrefix(name) {
				var tok pyToken
				tok, i = lexPyString(src, i, name)
				tokens = append(tokens, tok)
				continue
			}
			tokens = append(tokens, pyToken{kind: pyName, text: name})
		case c >= '0' && c <= '9':
			start := i
			for i < len(src) && (isPyNameByte(src[i], false) || src[i] == '.') {
				i++
			}
			tokens = append(tokens, pyToken{kind: pyOther, text: src[start:i]})
		case c == '=' && i+1 < len(src) && src[i+1] == '=':
			tokens = append(tokens, pyToken{kind: pyOther, text: "=="})
			i += 2
		default:
			tok := pyToken{kind: pyOther, text: string(c)}
			depth = max(depth+bracketDelta(tok), 0)
			tokens = append(tokens, tok)
			i++
		}
	}
	return tokens
}

func isPyStringPrefix(name string) bool {
	switch strings.ToLower(name) {
	case "r", "u", "b", "f", "t", "br", "rb", "fr", "rf", "tr", "rt":
		return true
	}
	return false
}

// lexPyString reads the string literal whose opening quote is at
// src[i], in single or triple quotes, and returns it with the index
// just past it. An unterminated literal runs to the end of src and is
// marked unsure.
func lexPyString(src string, i int, prefix string) (pyToken, int) {
	quote := src[i : i+1]
	if strings.HasPrefix(src[i:], strings.Repeat(quote, 3)) {
		quote = strings.Repeat(quote, 3)
	}
	start := i + len(quote)
	lower := strings.ToLower(prefix)
	var end int
	unsure := false
	if strings.ContainsAny(lower, "ft") {
		scan := formattedScanner{src: src, quote: quote}
		end = scan.body(start)
		unsure = scan.unsure
	} else {
		end = scanStringBody(src, start, quote)
	}
	tok := pyToken{
		kind:   pyString,
		text:   prefix + src[i:min(end+len(quote), len(src))],
		value:  src[start:end],
		plain:  lower == "" || lower == "u" || lower == "r",
		unsure: unsure || !strings.HasPrefix(src[end:], quote),
	}
	return tok, min(end+len(quote), len(src))
}

// scanStringBody returns the index of the quote closing a string body
// that starts at src[j], or where an unterminated body ends.
func scanStringBody(src string, j int, quote string) int {
	for j < len(src) && !strings.HasPrefix(src[j:], quote) {
		if src[j] == '\\' {
			j++
		} else if len(quote) == 1 && src[j] == '\n' {
			break
		}
		j++
	}
	return min(j, len(src))
}

// formattedScanner scans the body of an f-string or t-string closed
// by quote. A quote inside a replacement field opens a nested string,
// as Python 3.12 allows, rather than closing the literal. A backslash
// never escapes a brace. unsure records a body, field or nested string
// whose end the scanner could not place.
type formattedScanner struct {
	src    string
	quote  string
	unsure bool
}

// skipBackslash returns the index past a backslash at src[j] and the
// character it escapes, leaving a brace for the field scan.
func (s *formattedScanner) skipBackslash(j int) int {
	if j+1 < len(s.src) && (s.src[j+1] == openBrace || s.src[j+1] == closeBrace) {
		return j + 1
	}
	return j + 2
}

// body returns the index of the quote closing a body that starts at
// src[j], or where an unterminated body ends.
func (s *formattedScanner) body(j int) int {
	src := s.src
	for j < len(src) && !strings.HasPrefix(src[j:], s.quote) {
		switch {
		case src[j] == '\\':
			j = s.skipBackslash(j)
		case len(s.quote) == 1 && src[j] == '\n':
			return j
		case src[j] == openBrace && j+1 < len(src) && src[j+1] == openBrace:
			j += 2
		case src[j] == openBrace:
			j = s.field(j + 1)
		default:
			j++
		}
	}
	return min(j, len(src))
}

// field skips the expression of a replacement field that starts at
// src[j] and returns the index just past its closing brace. A colon
// outside brackets starts the format spec.
func (s *formattedScanner) field(j int) int {
	src := s.src
	depth := 0
	for j < len(src) {
		c := src[j]
		switch {
		case c == closeBrace && depth == 0:
			return j + 1
		case c == ':' && depth == 0:
			return s.spec(j + 1)
		case c == '"', c == '\'':
			var tok pyToken
			tok, j = lexPyString(src, j, "")
			s.unsure = s.unsure || tok.unsure
		case isPyNameByte(c, true):
			start := j
			for j < len(src) && isPyNameByte(src[j], false) {
				j++
			}
			if j < len(src) && (src[j] == '"' || src[j] == '\'') && isPyStringPrefix(src[start:j]) {
				var tok pyToken
				tok, j = lexPyString(src, j, src[start:j])
				s.unsure = s.unsure || tok.unsure
			}
		case c == '#':
			for j < len(src) && src[j] != '\n' {
				j++
			}
		default:
			depth = max(depth+bracketDelta(pyToken{kind: pyOther, text: string(c)}), 0)
			j++
		}
	}
	s.unsure = true
	return j
}

// spec skips the format spec of a replacement field, which is literal
// text that may hold nested fields, and returns the index just past
// the field's closing brace.
func (s *formattedScanner) spec(j int) int {
	src := s.src
	for j < len(src) && !strings.HasPrefix(src[j:], s.quote) {
		switch {
		case src[j] == closeBrace:
			return j + 1
		case src[j] == openBrace:
			j = s.field(j + 1)
		case src[j] == '\\':
			j = s.skipBackslash(j)
		case len(s.quote) == 1 && src[j] == '\n':
			s.unsure = true
			return j
		default:
			j++
		}
	}
	s.unsure = true
	return min(j, len(src))
}

// setupCallKeywords returns the keyword arguments of the first
// setup(...) or <name>.setup(...) call in a setup.py source that
// starts a logical line, at any indentation, and whose value is one
// plain string literal holding no backslash. A call inside another
// statement or expression, such as an assignment, a decorator, an
// argument of another call or a case pattern, is skipped. A keyword of
// a call nested in that call, a lambda's default, a comment and a
// string literal never count. A source holding a string literal whose
// end the lexer cannot place gives no keywords at all.
func setupCallKeywords(src string) map[string]string {
	tokens := lexPython(src)
	for _, tok := range tokens {
		if tok.unsure {
			return map[string]string{}
		}
	}
	for i := 0; i+1 < len(tokens); i++ {
		if tokens[i].kind != pyName || tokens[i].text != "setup" || tokens[i+1].text != "(" {
			continue
		}
		lineStart := i
		if i >= 2 && tokens[i-1].text == "." && tokens[i-2].kind == pyName {
			lineStart = i - 2
		}
		if lineStart > 0 && tokens[lineStart-1].kind != pyNewline {
			continue
		}
		return callKeywords(tokens[i+2:])
	}
	return map[string]string{}
}

// callKeywords reads the arguments of a call whose opening parenthesis
// precedes tokens, up to its closing parenthesis. An argument ends at
// a comma outside brackets and outside a lambda's parameter list.
func callKeywords(tokens []pyToken) map[string]string {
	keywords := map[string]string{}
	addArgument := func(arg []pyToken) {
		if len(arg) < 3 || arg[0].kind != pyName || arg[1].kind != pyOther || arg[1].text != "=" {
			return
		}
		value := arg[2:]
		if _, seen := keywords[arg[0].text]; !seen && len(value) == 1 && value[0].kind == pyString && value[0].plain &&
			!strings.Contains(value[0].value, "\\") {
			keywords[arg[0].text] = value[0].value
		}
	}
	depth, lambdas, start := 0, 0, 0
	for i, tok := range tokens {
		switch {
		case depth == 0 && tok.kind == pyName && tok.text == "lambda":
			lambdas++
		case tok.kind != pyOther:
		case depth == 0 && tok.text == ")":
			addArgument(tokens[start:i])
			return keywords
		case depth == 0 && tok.text == ":" && lambdas > 0:
			lambdas--
		case depth == 0 && tok.text == "," && lambdas == 0:
			addArgument(tokens[start:i])
			start = i + 1
		default:
			depth = max(depth+bracketDelta(tok), 0)
		}
	}
	addArgument(tokens[start:])
	return keywords
}

func bracketDelta(tok pyToken) int {
	if tok.kind != pyOther {
		return 0
	}
	switch tok.text {
	case "(", "[", "{":
		return 1
	case ")", "]", "}":
		return -1
	}
	return 0
}
