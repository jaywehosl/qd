package auth

type Kind uint8

const (
	User Kind = iota
	Node
	Admin
)

type Token struct {
	Raw  string
	Kind Kind
}

type Authenticator interface {
	Verify(raw string) (Token, bool)
}
