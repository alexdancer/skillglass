package acceptance_test

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type ale24Fixture struct {
	fixture
	rootA          string
	rootB          string
	sharedDir      string
	sharedManifest string
}

func TestALE24GroupsOnlyProvenDirectoryReferences(t *testing.T) {
	fx := newALE24Fixture(t)
	before := treeManifest(t, fx.base)
	result := fx.run(
		t,
		"list",
		"--root", fx.rootA,
		"--project", fx.project,
		"--project-root", fx.rootB,
		"--json",
	)
	assertExit(t, result, 0)
	var inventory publicList
	decodeJSON(t, result.stdout, &inventory)
	if inventory.Operation != "list" || inventory.Scope.Completeness != "complete" {
		t.Fatalf("identity inventory = %#v", inventory)
	}
	if len(inventory.Scope.Roots) != 2 ||
		inventory.Scope.Roots[0].Context != "custom" ||
		inventory.Scope.Roots[1].Context != "project" {
		t.Fatalf("root provenance = %#v", inventory.Scope.Roots)
	}

	shared := sourcesNamed(inventory.Sources, "shared-directory")
	if len(shared) != 1 || len(shared[0].ReferenceIDs) != 2 {
		t.Fatalf("directory aliases = %#v", shared)
	}
	sharedReferences := referencesForSource(inventory.References, shared[0].ID)
	if len(sharedReferences) != 2 ||
		sharedReferences[0].DiscoveredPath == sharedReferences[1].DiscoveredPath ||
		sharedReferences[0].ResolvedPath != canonicalPath(t, fx.sharedDir) ||
		sharedReferences[1].ResolvedPath != canonicalPath(t, fx.sharedDir) {
		t.Fatalf("shared directory references = %#v", sharedReferences)
	}

	manifestAliases := sourcesNamed(inventory.Sources, "shared-manifest-name")
	if len(manifestAliases) != 2 || manifestAliases[0].ID == manifestAliases[1].ID {
		t.Fatalf("manifest aliases were grouped: %#v", manifestAliases)
	}
	for _, source := range manifestAliases {
		if !source.Manifest.IsSymlink || source.Manifest.ResolvedPath != canonicalPath(t, fx.sharedManifest) {
			t.Fatalf("shared manifest provenance = %#v", source.Manifest)
		}
		read := fx.run(
			t,
			"read",
			"--root", fx.rootA,
			"--project", fx.project,
			"--project-root", fx.rootB,
			"--json",
			source.ID,
		)
		assertExit(t, read, 0)
		if !bytes.Contains(read.stdout, []byte("a-only.txt")) &&
			!bytes.Contains(read.stdout, []byte("b-only.txt")) {
			t.Fatalf("manifest alias resources missing: %s", read.stdout)
		}
	}
	if len(sourcesNamed(inventory.Sources, "equal-copy-name")) != 2 {
		t.Fatalf("independent equal copies were grouped: %#v", inventory.Sources)
	}
	if len(sourcesNamed(inventory.Sources, "duplicate-name")) != 2 {
		t.Fatalf("divergent duplicate names were grouped: %#v", inventory.Sources)
	}
	assertInformationalCode(t, inventory.Problems, "duplicate-name")
	assertInformationalCode(t, inventory.Problems, "shared-manifest")
	if !bytes.Equal(before, treeManifest(t, fx.base)) {
		t.Fatal("identity inventory mutated fixture")
	}
}

func TestALE24FiltersUseReferenceProvenanceWithANDSemantics(t *testing.T) {
	fx := newALE24Fixture(t)
	baseArgs := []string{
		"list",
		"--root", fx.rootA,
		"--project", fx.project,
		"--project-root", fx.rootB,
	}

	customAtProjectRoot := fx.run(t, append(baseArgs,
		"--source-root", fx.rootB,
		"--context", "custom",
		"--json",
	)...)
	assertExit(t, customAtProjectRoot, 0)
	var empty publicList
	decodeJSON(t, customAtProjectRoot.stdout, &empty)
	if len(empty.Sources) != 0 || empty.Filters.SourceRoot != fx.rootB || empty.Filters.Context != "custom" {
		t.Fatalf("AND filter result = %#v", empty)
	}

	sharedManifestAtProjectRoot := fx.run(t, append(baseArgs,
		"--source-root", fx.rootB,
		"--context", "project",
		"--problem", "shared-manifest",
		"--json",
	)...)
	assertExit(t, sharedManifestAtProjectRoot, 0)
	var projectAlias publicList
	decodeJSON(t, sharedManifestAtProjectRoot.stdout, &projectAlias)
	if len(projectAlias.Sources) != 1 || projectAlias.Sources[0].Name != "shared-manifest-name" {
		t.Fatalf("shared manifest AND filter = %#v", projectAlias.Sources)
	}
	if len(projectAlias.References) != 1 || !projectAlias.References[0].MatchesFilters || projectAlias.References[0].Context != "project" {
		t.Fatalf("matching references = %#v", projectAlias.References)
	}

	duplicates := fx.run(t, append(baseArgs,
		"--search", "duplicate-description",
		"--problem", "duplicate-name",
		"--json",
	)...)
	assertExit(t, duplicates, 0)
	var duplicateMatches publicList
	decodeJSON(t, duplicates.stdout, &duplicateMatches)
	if got := sourceNames(duplicateMatches.Sources); strings.Join(got, ",") != "duplicate-name,duplicate-name" {
		t.Fatalf("duplicate search AND filter = %v", got)
	}

	dual := fx.run(
		t,
		"list",
		"--root", fx.rootA,
		"--project", fx.project,
		"--project-root", fx.rootA,
		"--context", "project",
		"--json",
	)
	assertExit(t, dual, 0)
	var projectView publicList
	decodeJSON(t, dual.stdout, &projectView)
	if len(projectView.Sources) == 0 {
		t.Fatal("same normalized root lost its project provenance")
	}
	dualCustom := fx.run(
		t,
		"list",
		"--root", fx.rootA,
		"--project", fx.project,
		"--project-root", fx.rootA,
		"--context", "custom",
		"--json",
	)
	assertExit(t, dualCustom, 0)
	var customView publicList
	decodeJSON(t, dualCustom.stdout, &customView)
	if len(customView.Sources) != len(projectView.Sources) {
		t.Fatalf("same-root contexts differ: project=%d custom=%d", len(projectView.Sources), len(customView.Sources))
	}
	for _, source := range projectView.Sources {
		if source.Ownership.State != "unknown" || source.Availability.State != "unknown" {
			t.Fatalf("reference provenance invented native evidence: %#v", source)
		}
	}

	withoutProject := fx.run(t, "list", "--project-root", fx.rootB, "--json")
	assertExit(t, withoutProject, 2)
	unselectedRoot := fx.run(t, append(baseArgs, "--source-root", fx.emptyRoot, "--json")...)
	assertExit(t, unselectedRoot, 2)
}

func TestALE24ProblemsRemainSearchableWithUsablePartialInventory(t *testing.T) {
	fx := newALE24Fixture(t)
	paths := fx.addProblems(t)
	before := treeManifest(t, fx.base)
	args := []string{
		"--root", fx.rootA,
		"--root", paths.missingRoot,
		"--root", paths.brokenRoot,
		"--root", paths.cycleRoot,
		"--project", fx.project,
		"--project-root", fx.rootB,
		"--json",
	}
	result := fx.run(t, append([]string{"problems"}, args...)...)
	assertExit(t, result, 1)
	var inventory publicList
	decodeJSON(t, result.stdout, &inventory)
	if inventory.Operation != "problems" || inventory.Scope.Completeness != "incomplete" {
		t.Fatalf("problem inventory = %#v", inventory)
	}
	for _, code := range []string{
		"root-missing",
		"broken-reference",
		"symlink-cycle",
		"unreadable-manifest",
		"malformed-manifest",
	} {
		if !hasProblemCode(inventory.Problems, code) {
			t.Fatalf("problem code %q missing from %#v", code, inventory.Problems)
		}
	}
	for _, problem := range inventory.Problems {
		if problem.Path == "" || problem.Summary == "" {
			t.Fatalf("problem lost useful path or reason: %#v", problem)
		}
	}

	filtered := fx.run(t, append([]string{
		"problems",
		"--problem", "malformed-manifest",
		"--search", "malformed",
	}, args...)...)
	assertExit(t, filtered, 1)
	var malformed publicList
	decodeJSON(t, filtered.stdout, &malformed)
	if len(malformed.Problems) != 1 || malformed.Problems[0].Code != "malformed-manifest" {
		t.Fatalf("problem filters = %#v", malformed.Problems)
	}

	list := fx.run(t, append([]string{"list"}, args...)...)
	assertExit(t, list, 1)
	var sources publicList
	decodeJSON(t, list.stdout, &sources)
	normal := sourceNamed(t, sources.Sources, "normal-a")
	read := fx.run(t, append([]string{"read"}, append(args[:len(args)-1], normal.ID)...)...)
	assertExit(t, read, 1)
	if !bytes.Contains(read.stdout, []byte("NORMAL_A_INSTRUCTIONS")) {
		t.Fatalf("healthy source was not readable from partial inventory: %s", read.stdout)
	}
	if !bytes.Equal(before, treeManifest(t, fx.base)) {
		t.Fatal("problem inventory mutated fixture")
	}
}

func TestALE24FreshInvocationRetainsObjectIdentityAcrossReferenceChanges(t *testing.T) {
	fx := newALE24Fixture(t)
	args := []string{
		"list",
		"--root", fx.rootA,
		"--project", fx.project,
		"--project-root", fx.rootB,
		"--json",
	}
	first := fx.run(t, args...)
	assertExit(t, first, 0)
	var before publicList
	decodeJSON(t, first.stdout, &before)
	shared := sourceNamed(t, before.Sources, "shared-directory")

	writeSkill(t, filepath.Join(fx.rootA, "00-new"), "new-source", "lexically earlier", "NEW_INSTRUCTIONS")
	writeSkill(t, filepath.Join(fx.rootA, "normal"), "normal-a", "description after edit", "NORMAL_A_INSTRUCTIONS")
	if err := os.Remove(filepath.Join(fx.rootA, "directory-alias-one")); err != nil {
		t.Fatal(err)
	}
	retarget := filepath.Join(fx.base, "allowed", "outside", "retarget")
	writeSkill(t, retarget, "retargeted-source", "new target", "RETARGETED")
	if err := os.Symlink(filepath.Join("..", "..", "outside", "retarget"), filepath.Join(fx.rootA, "directory-alias-one")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", "outside", "missing-after-refresh"), filepath.Join(fx.rootA, "new-broken")); err != nil {
		t.Fatal(err)
	}

	second := fx.run(t, args...)
	assertExit(t, second, 1)
	var refreshed publicList
	decodeJSON(t, second.stdout, &refreshed)
	stillShared := sourceNamed(t, refreshed.Sources, "shared-directory")
	if stillShared.ID != shared.ID || len(stillShared.ReferenceIDs) != 1 {
		t.Fatalf("surviving alias did not preserve selected object: before=%#v after=%#v", shared, stillShared)
	}
	if sourceNamed(t, refreshed.Sources, "normal-a").Description != "description after edit" {
		t.Fatal("fresh invocation did not observe manifest edit")
	}
	_ = sourceNamed(t, refreshed.Sources, "new-source")
	_ = sourceNamed(t, refreshed.Sources, "retargeted-source")
}

type ale24ProblemPaths struct {
	missingRoot string
	brokenRoot  string
	cycleRoot   string
}

func newALE24Fixture(t *testing.T) ale24Fixture {
	t.Helper()
	base := newFixture(t)
	fx := ale24Fixture{
		fixture:        base,
		rootA:          filepath.Join(base.base, "allowed", "roots", "ale24-a"),
		rootB:          filepath.Join(base.base, "allowed", "roots", "ale24-b"),
		sharedDir:      filepath.Join(base.base, "allowed", "outside", "shared-directory"),
		sharedManifest: filepath.Join(base.base, "allowed", "outside", "shared-manifest", "SKILL.md"),
	}
	for _, directory := range []string{fx.rootA, fx.rootB, fx.sharedDir, filepath.Dir(fx.sharedManifest)} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeSkill(t, fx.sharedDir, "shared-directory", "two directory aliases", "SHARED_DIRECTORY_INSTRUCTIONS")
	writeFile(t, filepath.Join(fx.sharedDir, "references", "shared.txt"), []byte("SHARED_RESOURCE"), 0o644)
	for _, link := range []struct {
		root string
		name string
	}{
		{fx.rootA, "directory-alias-one"},
		{fx.rootB, "directory-alias-two"},
	} {
		if err := os.Symlink(filepath.Join("..", "..", "outside", "shared-directory"), filepath.Join(link.root, link.name)); err != nil {
			t.Fatal(err)
		}
	}

	writeFile(t, fx.sharedManifest, []byte(
		"---\nname: shared-manifest-name\ndescription: manifest-only aliases\n---\nSHARED_MANIFEST_INSTRUCTIONS\n",
	), 0o644)
	for _, alias := range []struct {
		root     string
		name     string
		resource string
	}{
		{fx.rootA, "manifest-alias-one", "a-only.txt"},
		{fx.rootB, "manifest-alias-two", "b-only.txt"},
	} {
		directory := filepath.Join(alias.root, alias.name)
		if err := os.MkdirAll(filepath.Join(directory, "references"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join("..", "..", "..", "outside", "shared-manifest", "SKILL.md"), filepath.Join(directory, "SKILL.md")); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(directory, "references", alias.resource), []byte(alias.resource), 0o644)
	}

	equalManifest := "---\nname: equal-copy-name\ndescription: equal-copy-description\n---\nEQUAL_COPY_INSTRUCTIONS\n"
	for _, directory := range []string{filepath.Join(fx.rootA, "equal-copy-one"), filepath.Join(fx.rootB, "equal-copy-two")} {
		writeFile(t, filepath.Join(directory, "SKILL.md"), []byte(equalManifest), 0o644)
		writeFile(t, filepath.Join(directory, "references", "same.txt"), []byte("SAME_RESOURCE"), 0o644)
	}
	writeSkill(t, filepath.Join(fx.rootA, "duplicate-one"), "duplicate-name", "duplicate-description-one", "DUPLICATE_ONE")
	writeSkill(t, filepath.Join(fx.rootB, "duplicate-two"), "duplicate-name", "duplicate-description-two", "DUPLICATE_TWO")
	writeSkill(t, filepath.Join(fx.rootA, "normal"), "normal-a", "healthy source", "NORMAL_A_INSTRUCTIONS")
	return fx
}

func (fx ale24Fixture) addProblems(t *testing.T) ale24ProblemPaths {
	t.Helper()
	writeFile(t, filepath.Join(fx.rootA, "malformed", "SKILL.md"), []byte(
		"---\nname: [unterminated\nMALFORMED_ALE24\n",
	), 0o644)
	unreadable := filepath.Join(fx.rootA, "unreadable-entry", "SKILL.md")
	writeFile(t, unreadable, []byte("---\nname: unreadable-entry\n---\nUNREADABLE\n"), 0o644)
	if err := os.Chmod(unreadable, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o644) })
	if file, err := os.Open(unreadable); err == nil {
		file.Close()
		t.Fatal("permission control could open unreadable entry")
	} else if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("permission control error = %v", err)
	}
	if err := os.Symlink(filepath.Join("..", "..", "outside", "does-not-exist"), filepath.Join(fx.rootA, "broken-entry")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("entry-cycle-b", filepath.Join(fx.rootA, "entry-cycle-a")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("entry-cycle-a", filepath.Join(fx.rootA, "entry-cycle-b")); err != nil {
		t.Fatal(err)
	}
	externalTarget := filepath.Join(fx.base, "allowed", "outside", "external-resources", "ale24.txt")
	writeFile(t, externalTarget, []byte("EXTERNAL_ALE24"), 0o644)
	if err := os.Symlink(
		filepath.Join("..", "..", "..", "..", "outside", "external-resources", "ale24.txt"),
		filepath.Join(fx.rootA, "normal", "external.txt"),
	); err != nil {
		t.Fatal(err)
	}
	brokenRoot := filepath.Join(fx.base, "allowed", "roots", "ale24-broken-root")
	if err := os.Symlink(filepath.Join("..", "outside", "missing-root-target"), brokenRoot); err != nil {
		t.Fatal(err)
	}
	cycleA := filepath.Join(fx.base, "allowed", "roots", "ale24-root-cycle-a")
	cycleB := filepath.Join(fx.base, "allowed", "roots", "ale24-root-cycle-b")
	if err := os.Symlink(filepath.Base(cycleB), cycleA); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Base(cycleA), cycleB); err != nil {
		t.Fatal(err)
	}
	return ale24ProblemPaths{
		missingRoot: filepath.Join(fx.base, "allowed", "roots", "ale24-missing-root"),
		brokenRoot:  brokenRoot,
		cycleRoot:   cycleA,
	}
}

func writeSkill(t *testing.T, directory, name, description, instructions string) {
	t.Helper()
	writeFile(t, filepath.Join(directory, "SKILL.md"), []byte(
		"---\nname: "+name+"\ndescription: "+description+"\n---\n"+instructions+"\n",
	), 0o644)
}

func sourcesNamed(sources []publicSource, name string) []publicSource {
	var matches []publicSource
	for _, source := range sources {
		if source.Name == name {
			matches = append(matches, source)
		}
	}
	return matches
}

func sourceNames(sources []publicSource) []string {
	names := make([]string, len(sources))
	for index, source := range sources {
		names[index] = source.Name
	}
	sort.Strings(names)
	return names
}

func referencesForSource(references []publicReference, sourceID string) []publicReference {
	var matches []publicReference
	for _, reference := range references {
		if reference.SourceID == sourceID {
			matches = append(matches, reference)
		}
	}
	return matches
}

func hasProblemCode(problems []publicProblem, code string) bool {
	for _, problem := range problems {
		if problem.Code == code {
			return true
		}
	}
	return false
}

func assertInformationalCode(t *testing.T, problems []publicProblem, code string) {
	t.Helper()
	for _, problem := range problems {
		if problem.Code == code {
			if problem.LimitsCoverage {
				t.Fatalf("informational %q problem limits coverage: %#v", code, problem)
			}
			return
		}
	}
	t.Fatalf("informational problem %q missing from %#v", code, problems)
}
