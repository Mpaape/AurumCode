package config

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path"
	"path/filepath"
	"sort"
)

// Files of a layer that its digest covers, relative to the layer root:
// exactly what a layer contributes or is checked for.
const (
	layerProfilesPath = ".aurumcode/profiles.yml"
	layerSkillsDir    = ".aurumcode/skills"
	layerSkillDoc     = "SKILL.md"
)

// layerDigest is a layer's content identity for the audit record, in the
// spirit of PolicyDigest: sha256 over config.yml, profiles.yml and every
// skill's SKILL.md that exists, in path order, each preceded by its
// NUL-delimited relative path so moving text between files changes it.
func layerDigest(root string) (string, error) {
	files, err := layerFiles(root)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	for _, rel := range files {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return "", err
		}
		h.Write([]byte("\x00" + rel + "\x00"))
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// layerFiles lists the layer's contributing files that exist, as
// slash-separated paths relative to root, sorted.
func layerFiles(root string) ([]string, error) {
	var files []string
	for _, rel := range []string{DefaultConfigPath, layerProfilesPath} {
		if regularFile(filepath.Join(root, filepath.FromSlash(rel))) {
			files = append(files, rel)
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(layerSkillsDir)))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, e := range entries {
		rel := path.Join(layerSkillsDir, e.Name(), layerSkillDoc)
		if e.IsDir() && regularFile(filepath.Join(root, filepath.FromSlash(rel))) {
			files = append(files, rel)
		}
	}
	sort.Strings(files)
	return files, nil
}

// regularFile reports whether p exists and is not a directory.
func regularFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
