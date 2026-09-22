package script

import "fmt"

// SyntaxError is a parse failure at a specific position in the source. Both
// Tokenize and Parse return this concrete type (never a bare fmt.Errorf) for
// every failure, so a caller that wants the position on its own - like
// internal/modcheck, which pairs it with the file it came from - can read
// Pos directly instead of re-parsing "at line %d col %d" back out of a
// string.
type SyntaxError struct {
	Pos Position
	Msg string // plain-language description, no position baked in
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("script: %s at line %d col %d", e.Msg, e.Pos.Line, e.Pos.Col)
}
