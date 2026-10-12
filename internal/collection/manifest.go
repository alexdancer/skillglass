package collection

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

const maxManifestBytes int64 = 4 << 20

func inspectManifest(bundleDirectory string) (Manifest, []Problem) {
	lexicalPath := filepath.Join(bundleDirectory, "SKILL.md")
	manifest := Manifest{Path: lexicalPath, Status: ManifestUnreadable}

	info, err := os.Lstat(lexicalPath)
	if err != nil {
		problem := makeProblem(
			ProblemUnreadableManifest,
			lexicalPath,
			"manifest could not be inspected",
			err,
			true,
		)
		return manifest, []Problem{problem}
	}

	resolvedPath := lexicalPath
	if info.Mode()&os.ModeSymlink != 0 {
		manifest.IsSymlink = true
		manifest.RawSymlinkTarget, err = os.Readlink(lexicalPath)
		if err != nil {
			problem := makeProblem(
				ProblemUnreadableManifest,
				lexicalPath,
				"manifest link could not be read",
				err,
				true,
			)
			return manifest, []Problem{problem}
		}
		resolvedPath, err = filepath.EvalSymlinks(lexicalPath)
		if err != nil {
			code := ProblemBrokenReference
			if isSymlinkCycle(err) {
				code = ProblemSymlinkCycle
			}
			problem := makeProblem(code, lexicalPath, "manifest link could not be resolved", err, true)
			return manifest, []Problem{problem}
		}
	}
	manifest.ResolvedPath = resolvedPath

	resolvedInfo, err := os.Stat(resolvedPath)
	if err != nil {
		problem := makeProblem(
			ProblemUnreadableManifest,
			lexicalPath,
			"manifest could not be inspected",
			err,
			true,
		)
		return manifest, []Problem{problem}
	}
	if !resolvedInfo.Mode().IsRegular() {
		err := fmt.Errorf("manifest is %s, not a regular file", resolvedInfo.Mode().Type())
		problem := makeProblem(
			ProblemUnreadableManifest,
			lexicalPath,
			"manifest is not a readable regular file",
			err,
			true,
		)
		return manifest, []Problem{problem}
	}
	if resolvedInfo.Size() > maxManifestBytes {
		err := fmt.Errorf("manifest is %d bytes; limit is %d bytes", resolvedInfo.Size(), maxManifestBytes)
		problem := makeProblem(
			ProblemManifestTooLarge,
			lexicalPath,
			"manifest exceeds the read limit",
			err,
			true,
		)
		return manifest, []Problem{problem}
	}

	file, err := os.Open(resolvedPath)
	if err != nil {
		problem := makeProblem(
			ProblemUnreadableManifest,
			lexicalPath,
			"manifest could not be opened",
			err,
			true,
		)
		return manifest, []Problem{problem}
	}
	defer file.Close()

	raw, err := io.ReadAll(io.LimitReader(file, maxManifestBytes+1))
	if err != nil {
		problem := makeProblem(
			ProblemUnreadableManifest,
			lexicalPath,
			"manifest could not be read",
			err,
			true,
		)
		return manifest, []Problem{problem}
	}
	if int64(len(raw)) > maxManifestBytes {
		err := fmt.Errorf("manifest exceeded the %d byte read limit", maxManifestBytes)
		problem := makeProblem(
			ProblemManifestTooLarge,
			lexicalPath,
			"manifest exceeds the read limit",
			err,
			true,
		)
		return manifest, []Problem{problem}
	}

	manifest.RawText = string(raw)
	frontmatter, instructions, err := splitManifest(raw)
	if err != nil {
		manifest.Status = ManifestMalformed
		problem := makeProblem(
			ProblemMalformedManifest,
			lexicalPath,
			"manifest frontmatter is malformed",
			err,
			true,
		)
		return manifest, []Problem{problem}
	}

	manifest.RawFrontmatter = string(frontmatter)
	manifest.Instructions = string(instructions)
	metadata, err := decodeMetadata(frontmatter)
	if err != nil {
		manifest.Status = ManifestMalformed
		problem := makeProblem(
			ProblemMalformedManifest,
			lexicalPath,
			"manifest frontmatter is malformed",
			err,
			true,
		)
		return manifest, []Problem{problem}
	}

	name, ok := metadata["name"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		manifest.Status = ManifestMalformed
		problem := makeProblem(
			ProblemMalformedManifest,
			lexicalPath,
			"manifest frontmatter is malformed",
			errors.New("name must be a non-empty string"),
			true,
		)
		return manifest, []Problem{problem}
	}
	description := ""
	if value, exists := metadata["description"]; exists {
		var descriptionOK bool
		description, descriptionOK = value.(string)
		if !descriptionOK {
			manifest.Status = ManifestMalformed
			problem := makeProblem(
				ProblemMalformedManifest,
				lexicalPath,
				"manifest frontmatter is malformed",
				errors.New("description must be a string"),
				true,
			)
			return manifest, []Problem{problem}
		}
	}

	manifest.Status = ManifestParsed
	manifest.Name = name
	manifest.Description = description
	manifest.Metadata = metadata
	return manifest, nil
}

func splitManifest(raw []byte) ([]byte, []byte, error) {
	firstEnd := bytes.IndexByte(raw, '\n')
	if firstEnd < 0 {
		return nil, nil, errors.New("frontmatter must begin with a --- line")
	}
	firstLine := bytes.TrimSuffix(raw[:firstEnd], []byte{'\r'})
	if !bytes.Equal(firstLine, []byte("---")) {
		return nil, nil, errors.New("frontmatter must begin with a --- line")
	}

	frontmatterStart := firstEnd + 1
	cursor := frontmatterStart
	for cursor <= len(raw) {
		lineEndRelative := bytes.IndexByte(raw[cursor:], '\n')
		lineEnd := len(raw)
		next := len(raw)
		if lineEndRelative >= 0 {
			lineEnd = cursor + lineEndRelative
			next = lineEnd + 1
		}
		line := bytes.TrimSuffix(raw[cursor:lineEnd], []byte{'\r'})
		if bytes.Equal(line, []byte("---")) || bytes.Equal(line, []byte("...")) {
			return raw[frontmatterStart:cursor], raw[next:], nil
		}
		if lineEndRelative < 0 {
			break
		}
		cursor = next
	}
	return nil, nil, errors.New("frontmatter has no closing --- line")
}

func decodeMetadata(raw []byte) (map[string]any, error) {
	var node yaml.Node
	if err := yaml.Unmarshal(raw, &node); err != nil {
		return nil, err
	}
	if len(node.Content) != 1 || node.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("frontmatter must be a YAML mapping")
	}

	metadata := make(map[string]any)
	if err := node.Content[0].Decode(&metadata); err != nil {
		return nil, err
	}
	return metadata, nil
}
