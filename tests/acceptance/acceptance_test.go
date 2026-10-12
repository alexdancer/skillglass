package acceptance_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

var (
	skillglassBinary     string
	isolationProbeBinary string
	repositoryRoot       string
	builtBinaryDir       string
	evidenceMu           sync.Mutex
	evidenceCases        []evidenceCase
)

type evidenceCase struct {
	ID               string   `json:"id"`
	Kind             string   `json:"kind"`
	Argv             []string `json:"argv"`
	WorkingDirectory string   `json:"working_directory"`
	EnvironmentKeys  []string `json:"environment_keys"`
	ExitCode         int      `json:"exit_code"`
	DurationMS       int64    `json:"duration_ms"`
	Stdout           string   `json:"stdout"`
	Stderr           string   `json:"stderr"`
}

func TestMain(testMain *testing.M) {
	_, currentFile, _, _ := runtime.Caller(0)
	repositoryRoot = filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	var err error
	builtBinaryDir, err = os.MkdirTemp("", "skillglass-acceptance-binary-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	goBinary := os.Getenv("GO_BIN")
	if goBinary == "" {
		goBinary = "/opt/homebrew/bin/go"
	}
	skillglassBinary = os.Getenv("SKILLGLASS_BIN")
	if skillglassBinary == "" {
		skillglassBinary = filepath.Join(builtBinaryDir, "skillglass")
		command := exec.Command(goBinary, "build", "-trimpath", "-o", skillglassBinary, "./cmd/skillglass")
		command.Dir = repositoryRoot
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		if err := command.Run(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.RemoveAll(builtBinaryDir)
			os.Exit(1)
		}
	}
	isolationProbeBinary = os.Getenv("SKILLGLASS_ISOLATION_PROBE")
	if isolationProbeBinary == "" {
		isolationProbeBinary = filepath.Join(builtBinaryDir, "isolationprobe")
		command := exec.Command(
			goBinary,
			"build",
			"-trimpath",
			"-o",
			isolationProbeBinary,
			"./tests/acceptance/cmd/isolationprobe",
		)
		command.Dir = repositoryRoot
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		if err := command.Run(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.RemoveAll(builtBinaryDir)
			os.Exit(1)
		}
	}
	code := testMain.Run()
	if err := writeEvidenceCaseIndex(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	os.RemoveAll(builtBinaryDir)
	os.Exit(code)
}

type fixture struct {
	base           string
	home           string
	xdgConfig      string
	xdgData        string
	xdgCache       string
	xdgState       string
	temporary      string
	project        string
	cleanRoot      string
	hostileRoot    string
	malformedRoot  string
	emptyRoot      string
	unreadableRoot string
	missingRoot    string
	marker         string
}

type commandResult struct {
	code     int
	stdout   []byte
	stderr   []byte
	duration time.Duration
}

type publicList struct {
	Scope struct {
		Project struct {
			Selected bool   `json:"selected"`
			Path     string `json:"path"`
		} `json:"project"`
		Roots []struct {
			Requested   string `json:"requested_path"`
			State       string `json:"state"`
			SourceCount int    `json:"source_count"`
		} `json:"roots"`
		Completeness string `json:"completeness"`
	} `json:"scope"`
	Sources  []publicSource `json:"sources"`
	Problems []struct {
		Code string `json:"code"`
		Path string `json:"path"`
	} `json:"problems"`
}

type publicRead struct {
	Source *publicSource `json:"source"`
	Error  string        `json:"error"`
}

type publicSource struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SourcePath  string `json:"source_path"`
	Ownership   struct {
		State string `json:"state"`
	} `json:"ownership"`
	Availability struct {
		State string `json:"state"`
	} `json:"availability"`
	Manifest struct {
		Status       string         `json:"status"`
		Metadata     map[string]any `json:"metadata"`
		Instructions string         `json:"instructions"`
		RawText      string         `json:"raw_text"`
	} `json:"manifest"`
	Resources []struct {
		Path       string `json:"path"`
		Kind       string `json:"kind"`
		Executable bool   `json:"executable"`
		LinkScope  string `json:"link_scope"`
	} `json:"resources"`
}

func TestListReadSearchAndScope(t *testing.T) {
	fx := newFixture(t)
	before := treeManifest(t, fx.base)

	list := fx.run(t, "list", "--root", fx.cleanRoot, "--project", fx.project, "--json")
	assertExit(t, list, 0)
	var inventory publicList
	decodeJSON(t, list.stdout, &inventory)
	if inventory.Scope.Completeness != "complete" {
		t.Fatalf("coverage = %q, want complete", inventory.Scope.Completeness)
	}
	if !inventory.Scope.Project.Selected || inventory.Scope.Project.Path != fx.project {
		t.Fatalf("project = %#v, want selected %q", inventory.Scope.Project, fx.project)
	}
	if len(inventory.Scope.Roots) != 1 || inventory.Scope.Roots[0].Requested != fx.cleanRoot {
		t.Fatalf("roots = %#v, want only %q", inventory.Scope.Roots, fx.cleanRoot)
	}
	if len(inventory.Sources) != 2 {
		t.Fatalf("sources = %d, want 2", len(inventory.Sources))
	}
	normal := sourceNamed(t, inventory.Sources, "normal-skill")
	if normal.Description != "description-only-literal" {
		t.Fatalf("description = %q", normal.Description)
	}
	if normal.Ownership.State != "unknown" || normal.Availability.State != "unknown" {
		t.Fatalf("evidence states = %q/%q, want unknown/unknown", normal.Ownership.State, normal.Availability.State)
	}
	if strings.Contains(string(list.stdout), "HOME_FALLBACK_CANARY") ||
		strings.Contains(string(list.stdout), "PROJECT_SCAN_CANARY") ||
		strings.Contains(string(list.stdout), "OUTSIDE_SIBLING_CANARY") {
		t.Fatal("list exposed a canary outside the selected root")
	}

	nameSearch := fx.run(t, "list", "--root", fx.cleanRoot, "--search", "normal-skill", "--json")
	assertExit(t, nameSearch, 0)
	var nameMatches publicList
	decodeJSON(t, nameSearch.stdout, &nameMatches)
	if len(nameMatches.Sources) != 1 || nameMatches.Sources[0].ID != normal.ID {
		t.Fatalf("name search returned %#v", nameMatches.Sources)
	}
	descriptionSearch := fx.run(
		t,
		"list",
		"--root",
		fx.cleanRoot,
		"--search",
		"description-only-literal",
		"--json",
	)
	assertExit(t, descriptionSearch, 0)
	var descriptionMatches publicList
	decodeJSON(t, descriptionSearch.stdout, &descriptionMatches)
	if len(descriptionMatches.Sources) != 1 || descriptionMatches.Sources[0].ID != normal.ID {
		t.Fatalf("description search returned %#v", descriptionMatches.Sources)
	}

	read := fx.run(
		t,
		"read",
		"--root",
		fx.cleanRoot,
		"--project",
		fx.project,
		"--json",
		normal.ID,
	)
	assertExit(t, read, 0)
	var detail publicRead
	decodeJSON(t, read.stdout, &detail)
	if detail.Source == nil {
		t.Fatalf("read source missing: %s", read.stdout)
	}
	for _, sentinel := range []string{
		"FIRST_INSTRUCTION_SENTINEL",
		"MIDDLE_INSTRUCTION_SENTINEL",
		"LAST_INSTRUCTION_SENTINEL",
	} {
		if !strings.Contains(detail.Source.Manifest.Instructions, sentinel) {
			t.Fatalf("instructions missing %q", sentinel)
		}
	}
	assertResource(t, detail.Source.Resources, "references/nested.txt", "file")
	assertResource(t, detail.Source.Resources, "assets/pixel.bin", "file")
	assertResource(t, detail.Source.Resources, "scripts/never-run.sh", "file")
	assertResource(t, detail.Source.Resources, "resources/never-open.fifo", "named-pipe")
	if got := detail.Source.Manifest.Metadata["category"]; got != "acceptance" {
		t.Fatalf("metadata category = %#v", got)
	}

	after := treeManifest(t, fx.base)
	if !bytes.Equal(before, after) {
		t.Fatalf("fixture tree changed\nbefore: %s\nafter: %s", before, after)
	}
	if _, err := os.Stat(fx.marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("script marker stat error = %v, want not exist", err)
	}
	writeEvidence(t, "tree/cli-before.json", before)
	writeEvidence(t, "tree/cli-after.json", after)
}

func TestEmptyMissingMixedAndNoImplicitRoots(t *testing.T) {
	fx := newFixture(t)
	help := fx.run(t, "--help")
	assertExit(t, help, 0)
	if !bytes.Contains(help.stdout, []byte("skillglass list --root ROOT")) {
		t.Fatalf("help output = %s", help.stdout)
	}
	empty := fx.run(t, "list", "--root", fx.emptyRoot, "--json")
	assertExit(t, empty, 0)
	var emptyInventory publicList
	decodeJSON(t, empty.stdout, &emptyInventory)
	if len(emptyInventory.Sources) != 0 || emptyInventory.Scope.Roots[0].State != "empty" {
		t.Fatalf("empty inventory = %#v", emptyInventory)
	}
	emptyHuman := fx.run(t, "list", "--root", fx.emptyRoot)
	assertExit(t, emptyHuman, 0)
	if !bytes.Contains(emptyHuman.stdout, []byte("Project: none selected")) {
		t.Fatalf("project omission is not visible: %s", emptyHuman.stdout)
	}

	missing := fx.run(t, "list", "--root", fx.missingRoot, "--json")
	assertExit(t, missing, 1)
	var missingInventory publicList
	decodeJSON(t, missing.stdout, &missingInventory)
	if len(missingInventory.Sources) != 0 ||
		missingInventory.Scope.Completeness != "incomplete" ||
		missingInventory.Scope.Roots[0].State != "missing" {
		t.Fatalf("missing inventory = %#v", missingInventory)
	}

	mixed := fx.run(t, "list", "--root", fx.cleanRoot, "--root", fx.missingRoot, "--json")
	assertExit(t, mixed, 1)
	var mixedInventory publicList
	decodeJSON(t, mixed.stdout, &mixedInventory)
	if len(mixedInventory.Sources) != 2 || mixedInventory.Scope.Completeness != "incomplete" {
		t.Fatalf("mixed inventory = %#v", mixedInventory)
	}

	noRoot := fx.run(t, "list", "--json")
	assertExit(t, noRoot, 2)
	if bytes.Contains(noRoot.stdout, []byte("HOME_FALLBACK_CANARY")) {
		t.Fatal("no-root invocation scanned isolated home")
	}
	projectOnly := fx.run(t, "list", "--project", fx.project, "--json")
	assertExit(t, projectOnly, 2)
	if bytes.Contains(projectOnly.stdout, []byte("PROJECT_SCAN_CANARY")) {
		t.Fatal("project-only invocation scanned project")
	}
	unknownSelection := fx.run(
		t,
		"read",
		"--root",
		fx.cleanRoot,
		"--json",
		"src_does_not_exist",
	)
	assertExit(t, unknownSelection, 1)
	var unknownRead publicRead
	decodeJSON(t, unknownSelection.stdout, &unknownRead)
	if unknownRead.Error == "" || unknownRead.Source != nil {
		t.Fatalf("unknown selection output = %#v", unknownRead)
	}
}

func TestUnreadableEmptyRootDoesNotFabricateSource(t *testing.T) {
	fx := newFixture(t)
	if err := os.Chmod(fx.unreadableRoot, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(fx.unreadableRoot, 0o755)
	})
	file, err := os.Open(fx.unreadableRoot)
	if err == nil {
		file.Close()
		t.Fatal("permission control could open mode-000 root")
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("permission control error = %v, want permission denied", err)
	}

	result := fx.run(t, "list", "--root", fx.unreadableRoot, "--json")
	assertExit(t, result, 1)
	var inventory publicList
	decodeJSON(t, result.stdout, &inventory)
	if len(inventory.Sources) != 0 {
		t.Fatalf("unreadable empty root fabricated sources: %#v", inventory.Sources)
	}
	if inventory.Scope.Roots[0].State != "unreadable" ||
		inventory.Scope.Roots[0].SourceCount != 0 {
		t.Fatalf("unreadable root report = %#v", inventory.Scope.Roots[0])
	}
	mixed := fx.run(
		t,
		"list",
		"--root",
		fx.cleanRoot,
		"--root",
		fx.missingRoot,
		"--root",
		fx.unreadableRoot,
		"--json",
	)
	assertExit(t, mixed, 1)
	var mixedInventory publicList
	decodeJSON(t, mixed.stdout, &mixedInventory)
	if len(mixedInventory.Sources) != 2 ||
		mixedInventory.Scope.Completeness != "incomplete" ||
		len(mixedInventory.Scope.Roots) != 3 {
		t.Fatalf("mixed missing/unreadable inventory = %#v", mixedInventory)
	}
}

func TestMalformedReadableManifestPreservesRawText(t *testing.T) {
	fx := newFixture(t)
	list := fx.run(t, "list", "--root", fx.malformedRoot, "--json")
	assertExit(t, list, 1)
	var inventory publicList
	decodeJSON(t, list.stdout, &inventory)
	if len(inventory.Sources) != 1 {
		t.Fatalf("malformed source count = %d, want 1", len(inventory.Sources))
	}
	malformed := inventory.Sources[0]
	if malformed.Manifest.Status != "malformed" ||
		!strings.Contains(malformed.Manifest.RawText, "MALFORMED_RAW_SENTINEL") {
		t.Fatalf("malformed manifest = %#v", malformed.Manifest)
	}
	read := fx.run(t, "read", "--root", fx.malformedRoot, "--json", malformed.ID)
	assertExit(t, read, 1)
	var detail publicRead
	decodeJSON(t, read.stdout, &detail)
	if detail.Source == nil ||
		!strings.Contains(detail.Source.Manifest.RawText, "MALFORMED_RAW_SENTINEL") {
		t.Fatalf("read did not preserve malformed text: %s", read.stdout)
	}
	human := fx.run(t, "read", "--root", fx.malformedRoot, malformed.ID)
	assertExit(t, human, 1)
	if !bytes.Contains(human.stdout, []byte("MALFORMED_RAW_SENTINEL")) ||
		!bytes.Contains(human.stdout, []byte("malformed-manifest")) {
		t.Fatalf("human malformed output = %s", human.stdout)
	}
}

func TestHostileTerminalDataIsEscapedForHumans(t *testing.T) {
	fx := newFixture(t)
	listJSON := fx.run(t, "list", "--root", fx.hostileRoot, "--json")
	assertExit(t, listJSON, 0)
	var inventory publicList
	decodeJSON(t, listJSON.stdout, &inventory)
	if len(inventory.Sources) != 1 {
		t.Fatalf("hostile source count = %d", len(inventory.Sources))
	}
	hostile := inventory.Sources[0]
	if !strings.Contains(hostile.Name, "\x1b[2J") ||
		!strings.Contains(hostile.Description, "\x1b]0;HOSTILE_TITLE\a") {
		t.Fatalf("JSON did not preserve hostile decoded metadata: %#v", hostile)
	}

	listHuman := fx.run(t, "list", "--root", fx.hostileRoot)
	assertExit(t, listHuman, 0)
	readHuman := fx.run(t, "read", "--root", fx.hostileRoot, hostile.ID)
	assertExit(t, readHuman, 0)
	for _, result := range []commandResult{listHuman, readHuman} {
		for _, forbidden := range [][]byte{{0x1b}, {0x07}, {0x0d}, {0x9b}} {
			if bytes.Contains(result.stdout, forbidden) || bytes.Contains(result.stderr, forbidden) {
				t.Fatalf("human output contains terminal control %x\nstdout=%q\nstderr=%q", forbidden, result.stdout, result.stderr)
			}
		}
	}
	if !bytes.Contains(readHuman.stdout, []byte(`\x1b[31m`)) ||
		!bytes.Contains(readHuman.stdout, []byte("LAST_HOSTILE_SENTINEL")) {
		t.Fatalf("safe hostile rendering missing evidence: %s", readHuman.stdout)
	}
}

func TestDirectoryObjectIdentitySurvivesRename(t *testing.T) {
	fx := newFixture(t)
	first := fx.run(t, "list", "--root", fx.cleanRoot, "--search", "normal-skill", "--json")
	assertExit(t, first, 0)
	var before publicList
	decodeJSON(t, first.stdout, &before)
	normal := sourceNamed(t, before.Sources, "normal-skill")

	oldPath := filepath.Join(fx.cleanRoot, "normal")
	newPath := filepath.Join(fx.cleanRoot, "renamed-normal")
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatal(err)
	}
	second := fx.run(t, "list", "--root", fx.cleanRoot, "--search", "normal-skill", "--json")
	assertExit(t, second, 0)
	var after publicList
	decodeJSON(t, second.stdout, &after)
	renamed := sourceNamed(t, after.Sources, "normal-skill")
	if renamed.ID != normal.ID {
		t.Fatalf("source ID changed across directory rename: %q -> %q", normal.ID, renamed.ID)
	}
	canonicalNewPath, err := filepath.EvalSymlinks(newPath)
	if err != nil {
		t.Fatal(err)
	}
	if renamed.SourcePath != canonicalNewPath {
		t.Fatalf("source path = %q, want %q", renamed.SourcePath, canonicalNewPath)
	}
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	base := t.TempDir()
	fx := fixture{
		base:           base,
		home:           filepath.Join(base, "allowed", "home"),
		xdgConfig:      filepath.Join(base, "allowed", "xdg-config"),
		xdgData:        filepath.Join(base, "allowed", "xdg-data"),
		xdgCache:       filepath.Join(base, "allowed", "xdg-cache"),
		xdgState:       filepath.Join(base, "allowed", "xdg-state"),
		temporary:      filepath.Join(base, "allowed", "tmp"),
		project:        filepath.Join(base, "allowed", "projects", "project-one"),
		cleanRoot:      filepath.Join(base, "allowed", "roots", "clean"),
		hostileRoot:    filepath.Join(base, "allowed", "roots", "hostile"),
		malformedRoot:  filepath.Join(base, "allowed", "roots", "malformed"),
		emptyRoot:      filepath.Join(base, "allowed", "roots", "empty"),
		unreadableRoot: filepath.Join(base, "allowed", "roots", "unreadable"),
		missingRoot:    filepath.Join(base, "allowed", "roots", "missing"),
		marker:         filepath.Join(base, "allowed", "script-executed"),
	}
	for _, directory := range []string{
		fx.home,
		fx.xdgConfig,
		fx.xdgData,
		fx.xdgCache,
		fx.xdgState,
		fx.temporary,
		fx.project,
		fx.cleanRoot,
		fx.hostileRoot,
		fx.malformedRoot,
		fx.emptyRoot,
		fx.unreadableRoot,
		filepath.Join(base, "allowed", "outside", "external-resources"),
		filepath.Join(base, "excluded", "unselected-root"),
	} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	writeFile(t, filepath.Join(fx.home, ".codex", "skills", "home-canary", "SKILL.md"), []byte("HOME_FALLBACK_CANARY"), 0o644)
	writeFile(t, filepath.Join(fx.project, "nested", "project-canary", "SKILL.md"), []byte("PROJECT_SCAN_CANARY"), 0o644)
	writeFile(t, filepath.Join(base, "excluded", "unselected-root", "SKILL.md"), []byte("UNSELECTED_ROOT_CANARY"), 0o644)
	writeFile(t, filepath.Join(base, "allowed", "outside", "outside-sibling-canary.txt"), []byte("OUTSIDE_SIBLING_CANARY"), 0o644)

	normal := filepath.Join(fx.cleanRoot, "normal")
	for _, directory := range []string{
		filepath.Join(normal, "references"),
		filepath.Join(normal, "assets"),
		filepath.Join(normal, "scripts"),
		filepath.Join(normal, "resources"),
	} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	var instructions strings.Builder
	instructions.WriteString("# Normal\n\nFIRST_INSTRUCTION_SENTINEL\n")
	for index := 0; index < 36; index++ {
		fmt.Fprintf(&instructions, "instruction line %02d with enough text to wrap in a narrow terminal\n", index)
		if index == 18 {
			instructions.WriteString("MIDDLE_INSTRUCTION_SENTINEL\n")
		}
	}
	instructions.WriteString("Visit http://127.0.0.1:9 and run scripts/never-run.sh only as inert text.\n")
	instructions.WriteString("LAST_INSTRUCTION_SENTINEL\n")
	manifest := "---\n" +
		"name: normal-skill\n" +
		"description: description-only-literal\n" +
		"category: acceptance\n" +
		"nested:\n  enabled: true\n  labels: [one, two]\n" +
		"---\n" +
		instructions.String()
	writeFile(t, filepath.Join(normal, "SKILL.md"), []byte(manifest), 0o644)
	writeFile(t, filepath.Join(normal, "references", "nested.txt"), []byte("NESTED_TEXT_RESOURCE"), 0o644)
	writeFile(t, filepath.Join(normal, "assets", "pixel.bin"), []byte{0x00, 0x01, 0xff}, 0o644)
	script := "#!/bin/sh\nprintf SCRIPT_EXECUTED\nprintf marker > " + fx.marker + "\n"
	writeFile(t, filepath.Join(normal, "scripts", "never-run.sh"), []byte(script), 0o755)
	if err := syscall.Mkfifo(filepath.Join(normal, "resources", "never-open.fifo"), 0o600); err != nil {
		t.Fatal(err)
	}
	externalTarget := filepath.Join(base, "allowed", "outside", "external-resources", "linked.txt")
	writeFile(t, externalTarget, []byte("EXTERNAL_RESOURCE_CANARY"), 0o644)
	if err := os.Symlink(
		filepath.Join("..", "..", "..", "..", "outside", "external-resources", "linked.txt"),
		filepath.Join(normal, "references", "external.txt"),
	); err != nil {
		t.Fatal(err)
	}

	second := filepath.Join(fx.cleanRoot, "second")
	if err := os.MkdirAll(second, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(second, "SKILL.md"), []byte(
		"---\nname: second-skill\ndescription: nonmatching description\n---\nSECOND_INSTRUCTIONS\n",
	), 0o644)

	hostile := filepath.Join(fx.hostileRoot, "hostile")
	if err := os.MkdirAll(hostile, 0o755); err != nil {
		t.Fatal(err)
	}
	hostileManifest := "---\n" +
		"name: \"hostile-name-START\\u001b[2J-END\"\n" +
		"description: \"description-START\\u001b]0;HOSTILE_TITLE\\u0007-END\"\n" +
		"---\n" +
		"BODY_START\x1b[31mBODY_END\rCR_END\aBELL_END\nLAST_HOSTILE_SENTINEL\n"
	writeFile(t, filepath.Join(hostile, "SKILL.md"), []byte(hostileManifest), 0o644)
	writeFile(t, filepath.Join(hostile, "asset-\x1b[31m.txt"), []byte("HOSTILE_FILENAME"), 0o644)

	malformed := filepath.Join(fx.malformedRoot, "malformed")
	if err := os.MkdirAll(malformed, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(malformed, "SKILL.md"), []byte(
		"---\nname: [unterminated\nMALFORMED_RAW_SENTINEL\n",
	), 0o644)
	return fx
}

func (fx fixture) run(t *testing.T, args ...string) commandResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, skillglassBinary, args...)
	command.Dir = fx.project
	command.Env = fx.environment()
	command.Stdin = bytes.NewReader(nil)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	started := time.Now()
	err := command.Run()
	duration := time.Since(started)
	if ctx.Err() != nil {
		t.Fatalf("command timed out: %s %s", skillglassBinary, strings.Join(args, " "))
	}
	code := 0
	if err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Fatalf("run command: %v", err)
		}
		code = exitError.ExitCode()
	}
	result := commandResult{code: code, stdout: stdout.Bytes(), stderr: stderr.Bytes(), duration: duration}
	recordEvidenceCommand(t, "public-cli", skillglassBinary, args, fx.project, result)
	return result
}

func (fx fixture) environment() []string {
	return []string{
		"HOME=" + fx.home,
		"XDG_CONFIG_HOME=" + fx.xdgConfig,
		"XDG_DATA_HOME=" + fx.xdgData,
		"XDG_CACHE_HOME=" + fx.xdgCache,
		"XDG_STATE_HOME=" + fx.xdgState,
		"TMPDIR=" + fx.temporary,
		"PATH=/usr/bin:/bin",
		"TERM=xterm-256color",
		"LC_ALL=C",
		"COLUMNS=80",
		"LINES=24",
	}
}

func sourceNamed(t *testing.T, sources []publicSource, name string) publicSource {
	t.Helper()
	for _, source := range sources {
		if source.Name == name {
			return source
		}
	}
	t.Fatalf("source %q not found in %#v", name, sources)
	return publicSource{}
}

func assertResource(t *testing.T, resources []struct {
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	Executable bool   `json:"executable"`
	LinkScope  string `json:"link_scope"`
}, path, kind string) {
	t.Helper()
	for _, resource := range resources {
		if resource.Path == path && resource.Kind == kind {
			return
		}
	}
	t.Fatalf("resource %q (%s) not found in %#v", path, kind, resources)
}

func assertExit(t *testing.T, result commandResult, want int) {
	t.Helper()
	if result.code != want {
		t.Fatalf(
			"exit = %d, want %d\nstdout:\n%s\nstderr:\n%s",
			result.code,
			want,
			result.stdout,
			result.stderr,
		)
	}
}

func decodeJSON(t *testing.T, raw []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, raw)
	}
}

func writeFile(t *testing.T, path string, content []byte, mode fs.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, mode); err != nil {
		t.Fatal(err)
	}
}

type treeEntry struct {
	Path   string `json:"path"`
	Mode   string `json:"mode"`
	Size   int64  `json:"size"`
	Target string `json:"target,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

func treeManifest(t *testing.T, root string) []byte {
	t.Helper()
	entries := make([]treeEntry, 0)
	err := filepath.WalkDir(root, func(path string, directoryEntry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		info, err := directoryEntry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entry := treeEntry{
			Path: filepath.ToSlash(relative),
			Mode: info.Mode().String(),
			Size: info.Size(),
		}
		if info.Mode()&os.ModeSymlink != 0 {
			entry.Target, err = os.Readlink(path)
			if err != nil {
				return err
			}
		} else if info.Mode().IsRegular() {
			content, readErr := os.ReadFile(path)
			if readErr == nil {
				sum := sha256.Sum256(content)
				entry.SHA256 = hex.EncodeToString(sum[:])
			}
		}
		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(entries, func(left, right int) bool {
		return entries[left].Path < entries[right].Path
	})
	encoded, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func recordEvidenceCommand(
	t *testing.T,
	kind string,
	executable string,
	args []string,
	workingDirectory string,
	result commandResult,
) {
	t.Helper()
	root := os.Getenv("SKILLGLASS_EVIDENCE")
	if root == "" {
		return
	}
	evidenceMu.Lock()
	defer evidenceMu.Unlock()
	id := fmt.Sprintf("%03d", len(evidenceCases)+1)
	directory := filepath.Join(root, "commands", id)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	stdoutRelative := filepath.ToSlash(filepath.Join("commands", id, "stdout"))
	stderrRelative := filepath.ToSlash(filepath.Join("commands", id, "stderr"))
	if err := os.WriteFile(filepath.Join(directory, "stdout"), result.stdout, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "stderr"), result.stderr, 0o644); err != nil {
		t.Fatal(err)
	}
	argv := append([]string{executable}, args...)
	evidenceCases = append(evidenceCases, evidenceCase{
		ID:               id,
		Kind:             kind,
		Argv:             argv,
		WorkingDirectory: workingDirectory,
		EnvironmentKeys: []string{
			"COLUMNS",
			"HOME",
			"LC_ALL",
			"LINES",
			"PATH",
			"TERM",
			"TMPDIR",
			"XDG_CACHE_HOME",
			"XDG_CONFIG_HOME",
			"XDG_DATA_HOME",
			"XDG_STATE_HOME",
		},
		ExitCode:   result.code,
		DurationMS: result.duration.Milliseconds(),
		Stdout:     stdoutRelative,
		Stderr:     stderrRelative,
	})
}

func writeEvidenceCaseIndex() error {
	root := os.Getenv("SKILLGLASS_EVIDENCE")
	if root == "" {
		return nil
	}
	evidenceMu.Lock()
	defer evidenceMu.Unlock()
	encoded, err := json.MarshalIndent(evidenceCases, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	return os.WriteFile(filepath.Join(root, "cases.json"), encoded, 0o644)
}
