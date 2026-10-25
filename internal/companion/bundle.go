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
const ConsolidationContractVersion = 1
const Name = "skillglass-review"
const ConsolidationName = "skillglass-consolidate"

//go:embed skillglass-review skillglass-consolidate
var bundle embed.FS

type File struct {
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Content string `json:"content"`
}

type Bundle struct {
	Name                         string `json:"name"`
	Version                      string `json:"version"`
	ContractVersion              int    `json:"review_contract_version,omitempty"`
	ConsolidationContractVersion int    `json:"consolidation_contract_version,omitempty"`
	Files                        []File `json:"files"`
}

// Bundled returns a fresh copy. Setup consumers install these exact bytes only.
func Bundled() Bundle {
	result, err := BundledNamed(Name)
	if err != nil {
		panic(err)
	}
	return result
}

// BundledNamed returns a fresh copy of an exact supported companion bundle.
func BundledNamed(name string) (Bundle, error) {
	contract := 0
	switch name {
	case Name:
		contract = ContractVersion
	case ConsolidationName:
		contract = ConsolidationContractVersion
	default:
		return Bundle{}, fmt.Errorf("unknown bundled companion %q", name)
	}
	result := Bundle{Name: name, Version: Version, Files: []File{}}
	if name == Name {
		result.ContractVersion = contract
	} else {
		result.ConsolidationContractVersion = contract
	}
	if err := fs.WalkDir(bundle, name, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		raw, err := bundle.ReadFile(path)
		if err != nil {
			return err
		}
		result.Files = append(result.Files, File{Path: path[len(name)+1:], SHA256: fmt.Sprintf("%x", sha256.Sum256(raw)), Content: string(raw)})
		return nil
	}); err != nil {
		return Bundle{}, err
	}
	sort.Slice(result.Files, func(i, j int) bool { return result.Files[i].Path < result.Files[j].Path })
	return result, nil
}
