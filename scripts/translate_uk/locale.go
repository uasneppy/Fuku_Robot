package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Entry is one top-level key of a locale file. A scalar has exactly one part; a
// YAML sequence (only misc_runs today) has one part per item and List set.
type Entry struct {
	Key   string
	Parts []string
	List  bool
}

// LoadLocale reads a locale file in file order. It walks yaml.Node instead of
// decoding into map[string]any because a map loses key order, and the draft must
// follow en.yml's order so diffs stay reviewable. Scalars of any tag load as their
// source text, which is how the runtime loader treats them too (it prints every
// leaf with fmt.Sprint); only nesting is refused, since the tool edits flat keys.
func LoadLocale(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: root element must be a mapping", path)
	}
	root := doc.Content[0]
	seen := make(map[string]bool, len(root.Content)/2)
	entries := make([]Entry, 0, len(root.Content)/2)
	for i := 0; i+1 < len(root.Content); i += 2 {
		keyNode, valNode := root.Content[i], root.Content[i+1]
		key := keyNode.Value
		if seen[key] {
			return nil, fmt.Errorf("%s: duplicate key %q (line %d)", path, key, keyNode.Line)
		}
		seen[key] = true
		switch valNode.Kind {
		case yaml.ScalarNode:
			entries = append(entries, Entry{Key: key, Parts: []string{valNode.Value}})
		case yaml.SequenceNode:
			parts := make([]string, 0, len(valNode.Content))
			for _, item := range valNode.Content {
				if item.Kind != yaml.ScalarNode {
					return nil, fmt.Errorf("%s: key %q has a list item that is not text (line %d)", path, key, item.Line)
				}
				parts = append(parts, item.Value)
			}
			entries = append(entries, Entry{Key: key, Parts: parts, List: true})
		default:
			return nil, fmt.Errorf("%s: key %q is nested or an alias; only text and lists of text are supported (line %d)", path, key, valNode.Line)
		}
	}
	return entries, nil
}

// WriteLocale writes entries to path through a temp file in the same directory and
// a rename, so a crash or a full disk never leaves a half-written locale that the
// bot would then fail to parse. Every string is emitted as a !!str node: that is
// what makes yaml.v3 quote "yes", "null" or "123" instead of changing their type.
func WriteLocale(path string, entries []Entry) error {
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, e := range entries {
		key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: e.Key}
		var val *yaml.Node
		if e.List {
			val = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			for _, p := range e.Parts {
				val.Content = append(val.Content, scalarNode(p))
			}
		} else {
			if len(e.Parts) != 1 {
				return fmt.Errorf("key %q: a text entry needs exactly one part, has %d", e.Key, len(e.Parts))
			}
			val = scalarNode(e.Parts[0])
		}
		root.Content = append(root.Content, key, val)
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return writeFileAtomic(path, buf.Bytes())
}

func scalarNode(s string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
	if bytes.ContainsRune([]byte(s), '\n') {
		// Literal blocks keep long help texts readable in git diffs; the emitter
		// falls back to a quoted scalar by itself when a block cannot represent s.
		n.Style = yaml.LiteralStyle
	}
	return n
}

func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return err
	}
	return nil
}
