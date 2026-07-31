package domain

import "strings"

func (a FB2Author) String() string {
	parts := []string{}
	if a.LastName != "" {
		parts = append(parts, a.LastName)
	}
	if a.FirstName != "" {
		parts = append(parts, a.FirstName)
	}
	if a.MiddleName != "" {
		parts = append(parts, a.MiddleName)
	}
	if len(parts) == 0 && a.Nickname != "" {
		return a.Nickname
	}
	return strings.Join(parts, " ")
}
