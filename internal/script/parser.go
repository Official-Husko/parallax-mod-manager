package script

import (
	"fmt"
	"strings"
)

// ValueKind identifies what kind of scalar (or block) a Value holds.
type ValueKind int

const (
	KindString ValueKind = iota
	KindNumber
	KindIdent // bare word: yes, no, tags, dotted names, @variable references
	KindBlock
)

// Operator is the token between a key and a value: "=", or one of the
// comparison operators (">=", "<=", ">", "<", "!=") used in trigger/condition
// contexts.
type Operator string

// Value is a single scalar or nested block.
type Value struct {
	Kind  ValueKind
	Raw   string // scalar text; unset when Kind == KindBlock
	Block *Block // set iff Kind == KindBlock
	Pos   Position
}

// Entry is one key/op/value triple, or - when Key is empty - a bare list item
// (e.g. one of several quoted strings inside a tags = { "a" "b" } block).
type Entry struct {
	Key   string
	Op    Operator
	Value Value
	Pos   Position
	// Offset/EndOffset span the entry's full source text (key through value,
	// or just the value for a bare list item), for on-demand re-read without
	// keeping the text itself resident in memory.
	Offset, EndOffset int
}

// Block is an ordered sequence of entries. Order is preserved because file
// order matters for later hashing/merge determinism.
type Block struct {
	Entries []Entry
}

// File is the result of parsing one Clausewitz script file.
type File struct {
	Root Block
	// Variables collects every "@name = value" definition found anywhere in
	// the file, keyed without the leading "@". This is a raw collection, not
	// substitution - nothing resolves @references against it.
	Variables map[string]Value
}

type parser struct {
	tokens []Token
	pos    int
}

// Parse parses a full Clausewitz script file into a generic block/key/value
// tree. It does not know about mod-domain concepts (Type/Id/definitions) -
// see internal/definition for that layer.
func Parse(src []byte) (*File, error) {
	toks, err := Tokenize(src)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: toks}
	root, err := p.parseBlockBody()
	if err != nil {
		return nil, err
	}
	if tok := p.peek(); tok.Kind != TokenEOF {
		return nil, &SyntaxError{Pos: tok.Pos, Msg: fmt.Sprintf("unexpected %q", tok.Text)}
	}

	f := &File{Root: root, Variables: map[string]Value{}}
	collectVariables(root, f.Variables)
	return f, nil
}

func (p *parser) peek() Token {
	if p.pos >= len(p.tokens) {
		return Token{Kind: TokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *parser) advance() Token {
	tok := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return tok
}

// parseBlockBody parses entries until it sees a '}' or EOF. The caller is
// responsible for consuming the matching brace (or checking for EOF at the
// top level).
func (p *parser) parseBlockBody() (Block, error) {
	var block Block
	for {
		switch p.peek().Kind {
		case TokenRBrace, TokenEOF:
			return block, nil
		}
		entry, err := p.parseEntry()
		if err != nil {
			return block, err
		}
		block.Entries = append(block.Entries, entry)
	}
}

// parseEntry parses either a "key op value" triple, or - when no operator
// follows the first token - a single bare value used as a key-less list item.
func (p *parser) parseEntry() (Entry, error) {
	startOffset := p.peek().Offset

	if p.peek().Kind == TokenLBrace {
		val, err := p.parseValue()
		if err != nil {
			return Entry{}, err
		}
		return Entry{Value: val, Pos: val.Pos, Offset: startOffset, EndOffset: p.lastEndOffset()}, nil
	}

	headText, headKind, headPos, err := p.parseScalarHead()
	if err != nil {
		return Entry{}, err
	}

	switch p.peek().Kind {
	case TokenEquals, TokenOperator:
		opTok := p.advance()
		val, err := p.parseValue()
		if err != nil {
			return Entry{}, err
		}
		return Entry{
			Key: headText, Op: Operator(opTok.Text), Value: val, Pos: headPos,
			Offset: startOffset, EndOffset: p.lastEndOffset(),
		}, nil
	default:
		return Entry{
			Value: Value{Kind: headKind, Raw: headText, Pos: headPos}, Pos: headPos,
			Offset: startOffset, EndOffset: p.lastEndOffset(),
		}, nil
	}
}

// lastEndOffset returns the end offset of the most recently consumed token.
func (p *parser) lastEndOffset() int {
	if p.pos == 0 {
		return 0
	}
	return p.tokens[p.pos-1].EndOffset
}

// parseValue parses a nested block ("{ ... }") or a single scalar.
func (p *parser) parseValue() (Value, error) {
	if tok := p.peek(); tok.Kind == TokenLBrace {
		p.advance()
		block, err := p.parseBlockBody()
		if err != nil {
			return Value{}, err
		}
		close := p.peek()
		if close.Kind != TokenRBrace {
			return Value{}, &SyntaxError{Pos: close.Pos, Msg: "expected '}'"}
		}
		p.advance()
		return Value{Kind: KindBlock, Block: &block, Pos: tok.Pos}, nil
	}

	text, kind, pos, err := p.parseScalarHead()
	if err != nil {
		return Value{}, err
	}
	return Value{Kind: kind, Raw: text, Pos: pos}, nil
}

// parseScalarHead consumes one scalar token (string, number, identifier, or
// an "@name" variable reference/definition) and returns its text and kind.
func (p *parser) parseScalarHead() (text string, kind ValueKind, pos Position, err error) {
	tok := p.peek()
	switch tok.Kind {
	case TokenAt:
		p.advance()
		ident := p.peek()
		if ident.Kind != TokenIdent {
			return "", 0, tok.Pos, &SyntaxError{Pos: tok.Pos, Msg: "expected identifier after '@'"}
		}
		p.advance()
		return "@" + ident.Text, KindIdent, tok.Pos, nil
	case TokenString:
		p.advance()
		return tok.Text, KindString, tok.Pos, nil
	case TokenNumber:
		p.advance()
		return tok.Text, KindNumber, tok.Pos, nil
	case TokenIdent:
		p.advance()
		return tok.Text, KindIdent, tok.Pos, nil
	default:
		return "", 0, tok.Pos, &SyntaxError{Pos: tok.Pos, Msg: fmt.Sprintf("unexpected token %q", tok.Text)}
	}
}

// collectVariables walks the whole tree recording every "@name = value"
// definition it finds, at any nesting depth.
func collectVariables(b Block, vars map[string]Value) {
	for _, e := range b.Entries {
		if name, ok := strings.CutPrefix(e.Key, "@"); ok {
			vars[name] = e.Value
		}
		if e.Value.Kind == KindBlock && e.Value.Block != nil {
			collectVariables(*e.Value.Block, vars)
		}
	}
}
