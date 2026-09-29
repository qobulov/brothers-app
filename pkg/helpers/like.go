package helpers

import "strings"

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// EscapeLike makes user input match literally inside a LIKE/ILIKE pattern.
// PostgreSQL uses backslash as the default LIKE escape character.
func EscapeLike(value string) string {
	return likeEscaper.Replace(value)
}
