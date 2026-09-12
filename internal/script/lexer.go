package script

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type lexer struct {
	src    []byte
	pos    int
	line   int
	col    int
	tokens []Token
}

// Tokenize converts Clausewitz script source into a flat token stream, terminated
// by a single TokenEOF. It never returns an empty slice.
func Tokenize(src []byte) ([]Token, error) {
	l := &lexer{src: src, pos: 0, line: 1, col: 1}
	for {
		l.skipWhitespaceAndComments()
		if l.pos >= len(l.src) {
			l.emit(TokenEOF, "", l.pos, l.line, l.col)
			return l.tokens, nil
		}
		if err := l.next(); err != nil {
			return nil, err
		}
	}
}

func (l *lexer) peek() byte {
	if l.pos >= len(l.src) {
		return 0
	}
	return l.src[l.pos]
}

func (l *lexer) peekAt(offset int) byte {
	if l.pos+offset >= len(l.src) {
		return 0
	}
	return l.src[l.pos+offset]
}

func (l *lexer) advance() byte {
	b := l.src[l.pos]
	l.pos++
	if b == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return b
}

// emit records a token spanning [offset, l.pos) - every call site invokes
// this only after fully consuming the token's source bytes, so l.pos is
// already the token's end offset at the time of the call.
func (l *lexer) emit(kind TokenKind, text string, offset, line, col int) {
	l.tokens = append(l.tokens, Token{
		Kind: kind, Text: text,
		Offset: offset, EndOffset: l.pos,
		Pos: Position{Line: line, Col: col},
	})
}

func (l *lexer) skipWhitespaceAndComments() {
	for l.pos < len(l.src) {
		switch l.peek() {
		case ' ', '\t', '\r', '\n':
			l.advance()
		case '#':
			for l.pos < len(l.src) && l.peek() != '\n' {
				l.advance()
			}
		default:
			return
		}
	}
}

func (l *lexer) next() error {
	offset, line, col := l.pos, l.line, l.col
	switch b := l.peek(); {
	case b == '{':
		l.advance()
		l.emit(TokenLBrace, "{", offset, line, col)
	case b == '}':
		l.advance()
		l.emit(TokenRBrace, "}", offset, line, col)
	case b == '@':
		l.advance()
		l.emit(TokenAt, "@", offset, line, col)
	case b == '=':
		l.advance()
		l.emit(TokenEquals, "=", offset, line, col)
	case b == '"':
		return l.lexString(offset, line, col)
	case b == '>' || b == '<' || b == '!':
		l.lexOperator(offset, line, col)
	case isNumberStart(b, l.peekAt(1)):
		l.lexNumber(offset, line, col)
	default:
		l.lexIdent(offset, line, col)
	}
	return nil
}

func (l *lexer) lexString(offset, line, col int) error {
	l.advance() // opening quote
	var sb strings.Builder
	for {
		if l.pos >= len(l.src) {
			return fmt.Errorf("script: unterminated string starting at line %d col %d", line, col)
		}
		switch b := l.peek(); {
		case b == '"':
			l.advance()
			l.emit(TokenString, sb.String(), offset, line, col)
			return nil
		case b == '\\' && (l.peekAt(1) == '"' || l.peekAt(1) == '\\'):
			l.advance()
			sb.WriteByte(l.advance())
		default:
			sb.WriteByte(l.advance())
		}
	}
}

func (l *lexer) lexOperator(offset, line, col int) {
	op := string(l.advance())
	if l.peek() == '=' {
		op += string(l.advance())
	}
	l.emit(TokenOperator, op, offset, line, col)
}

func isNumberStart(b, next byte) bool {
	if isDigit(b) {
		return true
	}
	return b == '-' && isDigit(next)
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func (l *lexer) lexNumber(offset, line, col int) {
	start := l.pos
	if l.peek() == '-' {
		l.advance()
	}
	for isDigit(l.peek()) {
		l.advance()
	}
	if l.peek() == '.' && isDigit(l.peekAt(1)) {
		l.advance()
		for isDigit(l.peek()) {
			l.advance()
		}
	}
	l.emit(TokenNumber, string(l.src[start:l.pos]), offset, line, col)
}

// isIdentStop reports whether b terminates a bare identifier run.
func isIdentStop(b byte) bool {
	switch b {
	case ' ', '\t', '\r', '\n', '{', '}', '"', '#', '@', '=', '<', '>', '!':
		return true
	default:
		return false
	}
}

func (l *lexer) lexIdent(offset, line, col int) {
	start := l.pos
	for l.pos < len(l.src) && !isIdentStop(l.peek()) {
		l.advance()
	}
	if l.pos == start {
		// Byte we don't otherwise recognize (e.g. stray multi-byte rune lead byte);
		// consume it as a one-rune identifier so the lexer always makes progress.
		_, size := utf8.DecodeRune(l.src[l.pos:])
		if size < 1 {
			size = 1
		}
		for i := 0; i < size && l.pos < len(l.src); i++ {
			l.advance()
		}
	}
	l.emit(TokenIdent, string(l.src[start:l.pos]), offset, line, col)
}
