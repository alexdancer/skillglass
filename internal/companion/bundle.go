// Package companion holds the portable skill shipped by this executable.
package companion

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
	"sort"
)

const Version = "1.0.0"
const ContractVersion = 1
const Name = "skillglass-review"

//go:embed skillglass-review
var bundle embed.FS

type File struct {
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Content string `json:"content"`
}

type Bundle struct {
	Name            string `json:"name"`
	Version         string `json:"version"`
	ContractVersion int    `json:"review_contract_version"`
	Files           []File `json:"files"`
}

// Bundled returns a fresh copy. Setup consumers install these exact bytes only.
func Bundled() Bundle {
	result := Bundle{Name: Name, Version: Version, ContractVersion: ContractVersion, Files: []File{}}
	fs.WalkDir(bundle, Name, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			panic(err)
		}
		if entry.IsDir() {
			return nil
		}
		raw, err := bundle.ReadFile(path)
		if err != nil {
			panic(err)
		}
		result.Files = append(result.Files, File{Path: path[len(Name)+1:], SHA256: fmt.Sprintf("%x", sha256.Sum256(raw)), Content: string(raw)})
		return nil
	})
	sort.Slice(result.Files, func(i, j int) bool { return result.Files[i].Path < result.Files[j].Path })
	return result
}
