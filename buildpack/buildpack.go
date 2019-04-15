package buildpack

import "strings"

type Buildpack struct {
	ID      string
	Latest  bool
	Dir     string
	Version string
}

func (b *Buildpack) EscapedID() string {
	return strings.Replace(b.ID, "/", "_", -1)
}
