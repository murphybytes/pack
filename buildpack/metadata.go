package buildpack

import (
	"strings"
)

type Buildpack struct {
	ID      string `toml:"id"`
	URI     string `toml:"uri"`
	Latest  bool   `toml:"latest"`
	Dir     string
	Version string
}

type TOML struct {
	BP struct {
		ID      string `toml:"id"`
		Version string `toml:"version"`
	} `toml:"buildpack"`
}


func (b *Buildpack) EscapedID() string {
	return strings.Replace(b.ID, "/", "_", -1)
}