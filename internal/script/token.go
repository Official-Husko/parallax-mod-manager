// Package script parses Clausewitz script - the key = value / nested { } block
// text format used by Paradox game and mod files. See docs/script-format.md.
package script

// TokenKind identifies the lexical category of a Token.
type TokenKind int

const (
	TokenEOF      TokenKind = iota
	TokenIdent              // bare word: yes, no, some_tag, dmm_mod.1
	TokenString             // "quoted string"
	TokenNumber             // 123, 1.5, -3
	TokenLBrace             // {
	TokenRBrace             // }
	TokenEquals             // =
	TokenOperator           // >=, <=, >, <, !=
	TokenAt                 // @ (variable reference/definition marker)
)

// Position identifies a location in the source text.
type Position struct {
	Line int
	Col  int
}

// Token is a single lexical token produced by Tokenize.
type Token struct {
	Kind      TokenKind
	Text      string // raw text (unescaped for strings, as written for everything else)
	Offset    int    // byte offset into the source where the token starts
	EndOffset int    // byte offset one past the token's last source byte
	Pos       Position
}
