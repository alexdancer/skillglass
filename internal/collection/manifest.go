package collection

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

const maxManifestBytes int64 = 4 << 20

func inspectManifest(bundleDirectory string) (Manifest, []Problem) {
	return inspectManifestFile(filepath.Join(bundleDirectory, "SKILL.md"), false)
}

func inspectManifestFile(lexicalPath string, pi bool) (Manifest, []Problem) {
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

	if !utf8.Valid(raw) {
		manifest.Status = ManifestMalformed
		manifest.RawBytesBase64 = base64.StdEncoding.EncodeToString(raw)
		problem := makeProblem(
			ProblemManifestInvalidEncoding,
			lexicalPath,
			"manifest is not valid UTF-8",
			errors.New("exact bytes are available via list/read --json at manifest.raw_bytes_base64"),
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
	metadata, nonFiniteValues, err := decodeMetadata(frontmatter)
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
	if pi && (!ok || name == "") {
		name, ok = filepath.Base(filepath.Dir(lexicalPath)), true
	}
	if !ok || (!pi && strings.TrimSpace(name) == "") {
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
	if len(nonFiniteValues) == 0 {
		return manifest, nil
	}
	problem := makeProblem(
		ProblemMetadataNonFinite,
		lexicalPath,
		"non-finite metadata values use tagged JSON representations",
		errors.New(strings.Join(nonFiniteValues, ", ")),
		false,
	)
	return manifest, []Problem{problem}
}

func normalizeMetadataValue(value any, path string) (any, []string, error) {
	switch typed := value.(type) {
	case map[any]any:
		mapping := make(map[string]any, len(typed))
		for key, item := range typed {
			text, ok := key.(string)
			if !ok {
				return nil, nil, fmt.Errorf("%s: mapping key must be a string, got %T", path, key)
			}
			mapping[text] = item
		}
		return normalizeMetadataValue(mapping, path)
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		normalized := make(map[string]any, len(typed))
		var values []string
		for _, key := range keys {
			item, nested, err := normalizeMetadataValue(typed[key], path+"."+key)
			if err != nil {
				return nil, nil, err
			}
			normalized[key] = item
			values = append(values, nested...)
		}
		return normalized, values, nil
	case []any:
		normalized := make([]any, len(typed))
		var values []string
		for index, item := range typed {
			normalizedItem, nested, err := normalizeMetadataValue(
				item,
				fmt.Sprintf("%s[%d]", path, index),
			)
			if err != nil {
				return nil, nil, err
			}
			normalized[index] = normalizedItem
			values = append(values, nested...)
		}
		return normalized, values, nil
	case float64:
		normalized, values := normalizeFloat(typed, path)
		return normalized, values, nil
	case float32:
		normalized, values := normalizeFloat(float64(typed), path)
		return normalized, values, nil
	case nil, string, bool, int, int64, uint64, time.Time:
		return value, nil, nil
	default:
		return nil, nil, fmt.Errorf("%s: unsupported metadata value %T", path, value)
	}
}

func normalizeFloat(value float64, path string) (any, []string) {
	label := ""
	switch {
	case math.IsNaN(value):
		label = "nan"
	case math.IsInf(value, 1):
		label = "positive-infinity"
	case math.IsInf(value, -1):
		label = "negative-infinity"
	default:
		return value, nil
	}
	return map[string]any{
		"kind":  "non-finite-float",
		"value": label,
	}, []string{path + "=" + label}
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

func validateMetadataNode(node *yaml.Node) error {
	for _, child := range node.Content {
		if err := validateMetadataNode(child); err != nil {
			return err
		}
	}
	if node.Kind == yaml.ScalarNode {
		var value any
		if err := node.Decode(&value); err != nil {
			return err
		}
		if text, ok := value.(string); ok && !utf8.ValidString(text) {
			return fmt.Errorf("line %d: metadata string must be valid UTF-8", node.Line)
		}
	}
	if node.Kind == yaml.MappingNode {
		keys := make(map[string]int, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index]
			if key.Kind == yaml.ScalarNode && key.Value == "<<" && key.ShortTag() == "!!merge" {
				continue
			}
			var value any
			if err := key.Decode(&value); err != nil {
				return err
			}
			text, ok := value.(string)
			if !ok {
				return fmt.Errorf("line %d: mapping key must be a string, got %T", key.Line, value)
			}
			if line, exists := keys[text]; exists {
				return fmt.Errorf("line %d: mapping key %q already defined at line %d", key.Line, text, line)
			}
			keys[text] = key.Line
		}
	}
	return nil
}

func decodeMetadata(raw []byte) (map[string]any, []string, error) {
	var node yaml.Node
	if err := yaml.Unmarshal(raw, &node); err != nil {
		return nil, nil, err
	}
	if len(node.Content) != 1 || node.Content[0].Kind != yaml.MappingNode {
		return nil, nil, errors.New("frontmatter must be a YAML mapping")
	}

	if err := validateMetadataNode(&node); err != nil {
		return nil, nil, err
	}
	var metadata any
	if err := node.Content[0].Decode(&metadata); err != nil {
		return nil, nil, err
	}
	normalized, values, err := normalizeMetadataValue(metadata, "$")
	if err != nil {
		return nil, nil, err
	}
	return normalized.(map[string]any), values, nil
}
