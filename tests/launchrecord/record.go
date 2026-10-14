package launchrecord

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

type File struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}
type Profile struct {
	Bytes         []byte `json:"bytes_base64"`
	SHA256        string `json:"sha256"`
	Path          string `json:"path,omitempty"`
	InheritedFrom string `json:"inherited_profile_sha256,omitempty"`
}
type Identity struct {
	ID          string   `json:"launch_id"`
	Argv        []string `json:"argv"`
	Cwd         string   `json:"cwd"`
	Environment []string `json:"environment"`
	Executables []File   `json:"executables"`
	Artifacts   []File   `json:"artifacts,omitempty"`
	Profile     *Profile `json:"profile,omitempty"`
}
type Record struct {
	Identity  Identity
	Directory string
}

func Digest(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func Identify(path string) (File, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return File{}, err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return File{}, err
	}
	raw, err := os.ReadFile(resolved)
	if err != nil {
		return File{}, err
	}
	return File{resolved, Digest(raw)}, nil
}
func Begin(command *exec.Cmd, directory string, inherited *Profile, artifacts ...string) (*Record, error) {
	if directory == "" || command.Env == nil {
		return nil, errors.New("launch identity needs evidence destination and finalized environment")
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	cwd := command.Dir
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	launchID := hex.EncodeToString(id)
	identity := Identity{ID: launchID, Argv: append([]string(nil), command.Args...), Cwd: cwd, Environment: append([]string(nil), command.Env...), Profile: inherited}
	paths := []string{command.Path}
	if filepath.Base(command.Path) == "sandbox-exec" {
		if len(command.Args) < 4 {
			return nil, errors.New("sandbox launch lacks policy or payload")
		}
		profile := &Profile{}
		switch command.Args[1] {
		case "-p":
			profile.Bytes = []byte(command.Args[2])
		case "-f":
			profile.Path = command.Args[2]
			var err error
			profile.Bytes, err = os.ReadFile(profile.Path)
			if err != nil {
				return nil, err
			}
		default:
			return nil, errors.New("sandbox launch lacks recognized policy")
		}
		profile.SHA256 = Digest(profile.Bytes)
		identity.Profile = profile
		paths = append(paths, command.Args[3])
	}
	for _, path := range paths {
		file, err := Identify(path)
		if err != nil {
			return nil, err
		}
		identity.Executables = append(identity.Executables, file)
	}
	for _, path := range artifacts {
		file, err := Identify(path)
		if err != nil {
			return nil, err
		}
		identity.Artifacts = append(identity.Artifacts, file)
	}
	record := &Record{identity, directory}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	if err := record.persist("invocation", identity); err != nil {
		return nil, err
	}
	return record, nil
}
func (record *Record) persist(kind string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(record.Directory, record.Identity.ID+"."+kind+".json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = file.Write(append(raw, '\n')); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
func (record *Record) Complete(err error, stdout, stderr []byte) error {
	code := 0
	message := ""
	if err != nil {
		code = -1
		message = err.Error()
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			code = exit.ExitCode()
		}
	}
	return record.CompleteHashes(code, message, Digest(stdout), Digest(stderr))
}
func (record *Record) CompleteHashes(code int, message, stdout, stderr string) error {
	return record.persist("completion", struct {
		ID     string `json:"launch_id"`
		Exit   int    `json:"exit_code"`
		Error  string `json:"error,omitempty"`
		Stdout string `json:"stdout_sha256"`
		Stderr string `json:"stderr_sha256"`
	}{record.Identity.ID, code, message, stdout, stderr})
}
