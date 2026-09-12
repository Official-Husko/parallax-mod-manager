package steam

import (
	"fmt"
	"strings"
)

// vdfNode is one parsed VDF block: flat key/value pairs plus nested blocks.
type vdfNode struct {
	values   map[string]string
	children map[string]*vdfNode
}

type vdfTokenKind int

const (
	vdfString vdfTokenKind = iota
	vdfLBrace
	vdfRBrace
)

type vdfToken struct {
	kind vdfTokenKind
	text string
}

// tokenizeVDF lexes Valve's small VDF key/value format: quoted strings,
// '{'/'}' for nested blocks, and "//" line comments.
func tokenizeVDF(data []byte) ([]vdfToken, error) {
	var toks []vdfToken
	i, n := 0, len(data)
	for i < n {
		switch b := data[i]; {
		case b == ' ' || b == '\t' || b == '\r' || b == '\n':
			i++
		case b == '/' && i+1 < n && data[i+1] == '/':
			for i < n && data[i] != '\n' {
				i++
			}
		case b == '{':
			toks = append(toks, vdfToken{kind: vdfLBrace})
			i++
		case b == '}':
			toks = append(toks, vdfToken{kind: vdfRBrace})
			i++
		case b == '"':
			i++
			var sb strings.Builder
			for i < n && data[i] != '"' {
				if data[i] == '\\' && i+1 < n {
					i++
				}
				sb.WriteByte(data[i])
				i++
			}
			if i >= n {
				return nil, fmt.Errorf("steam: unterminated string in VDF data")
			}
			i++ // closing quote
			toks = append(toks, vdfToken{kind: vdfString, text: sb.String()})
		default:
			return nil, fmt.Errorf("steam: unexpected byte %q in VDF data", b)
		}
	}
	return toks, nil
}

type vdfParser struct {
	tokens []vdfToken
	pos    int
}

func (p *vdfParser) peek() (vdfToken, bool) {
	if p.pos >= len(p.tokens) {
		return vdfToken{}, false
	}
	return p.tokens[p.pos], true
}

func (p *vdfParser) advance() (vdfToken, bool) {
	tok, ok := p.peek()
	if ok {
		p.pos++
	}
	return tok, ok
}

// parseVDF parses a full VDF document into its top-level keyed blocks.
func parseVDF(data []byte) (map[string]*vdfNode, error) {
	p := &vdfParser{}
	toks, err := tokenizeVDF(data)
	if err != nil {
		return nil, err
	}
	p.tokens = toks
	_, children, err := parseVDFEntries(p, false)
	if err != nil {
		return nil, err
	}
	return children, nil
}

// parseVDFEntries parses "key value" and "key { ... }" pairs until a closing
// '}' (if insideBlock) or EOF (top level).
func parseVDFEntries(p *vdfParser, insideBlock bool) (map[string]string, map[string]*vdfNode, error) {
	values := map[string]string{}
	children := map[string]*vdfNode{}
	for {
		tok, ok := p.peek()
		if !ok {
			if insideBlock {
				return nil, nil, fmt.Errorf("steam: unexpected end of VDF data, missing '}'")
			}
			return values, children, nil
		}
		if tok.kind == vdfRBrace {
			if !insideBlock {
				return nil, nil, fmt.Errorf("steam: unexpected '}' at top level")
			}
			p.advance()
			return values, children, nil
		}
		if tok.kind != vdfString {
			return nil, nil, fmt.Errorf("steam: expected a quoted key, got '{'")
		}
		key, _ := p.advance()

		valTok, ok := p.peek()
		if !ok {
			return nil, nil, fmt.Errorf("steam: expected a value or block after key %q", key.text)
		}
		switch valTok.kind {
		case vdfLBrace:
			p.advance()
			childValues, childChildren, err := parseVDFEntries(p, true)
			if err != nil {
				return nil, nil, err
			}
			children[key.text] = &vdfNode{values: childValues, children: childChildren}
		case vdfString:
			p.advance()
			values[key.text] = valTok.text
		default:
			return nil, nil, fmt.Errorf("steam: unexpected '}' immediately after key %q", key.text)
		}
	}
}
