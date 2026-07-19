package adk

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	adkskill "google.golang.org/adk/v2/tool/skilltoolset/skill"

	"github.com/jftrade/jftrade-main/pkg/besteffort"
	strategypinespec "github.com/jftrade/jftrade-main/pkg/strategy/pinespec"
)

func (r *SkillRegistry) installArchive(ctx context.Context, sourceURL string, body []byte) (Skill, error) {
	tempDir, err := os.MkdirTemp("", "jftrade-adk-skill-*")
	if err != nil {
		return Skill{}, err
	}
	defer func() { besteffort.LogError(os.RemoveAll(tempDir)) }()

	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return Skill{}, fmt.Errorf("parse skill archive: %w", err)
	}
	if err := extractSkillArchive(reader, tempDir); err != nil {
		return Skill{}, err
	}
	return r.installExtractedArchiveSkill(ctx, sourceURL, tempDir)
}

func extractSkillArchive(reader *zip.Reader, tempDir string) error {
	var extractedBytes uint64
	for _, file := range reader.File {
		if err := extractSkillArchiveFile(file, tempDir, &extractedBytes); err != nil {
			return err
		}
	}
	return nil
}

func extractSkillArchiveFile(file *zip.File, tempDir string, extractedBytes *uint64) error {
	name := path.Clean(strings.TrimSpace(file.Name))
	if name == "." {
		return nil
	}
	if strings.HasPrefix(name, "../") || strings.HasPrefix(name, "/") {
		return fmt.Errorf("skill archive contains unsafe path %q", file.Name)
	}
	targetPath := filepath.Join(tempDir, filepath.FromSlash(name))
	if file.FileInfo().IsDir() {
		return os.MkdirAll(targetPath, 0o755)
	}
	*extractedBytes += file.UncompressedSize64
	if *extractedBytes > maxSkillArchiveSize {
		return fmt.Errorf("skill archive exceeds %d bytes after extraction", maxSkillArchiveSize)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	return copySkillArchiveFile(file, targetPath)
}

func copySkillArchiveFile(file *zip.File, targetPath string) error {
	in, err := file.Open()
	if err != nil {
		return err
	}
	defer func() { besteffort.LogError(in.Close()) }()
	out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, io.LimitReader(in, int64(maxSkillArchiveSize)+1))
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func (r *SkillRegistry) installExtractedArchiveSkill(ctx context.Context, sourceURL string, tempDir string) (Skill, error) {
	fm, skillDoc, err := rewriteArchiveSkillDocument(tempDir, sourceURL)
	if err != nil {
		return Skill{}, err
	}
	if _, _, err := r.installSkillDirectory(fm.Name, filepath.Dir(skillDoc)); err != nil {
		return Skill{}, err
	}
	return r.loadInstalledArchiveSkill(ctx, fm.Name)
}

func rewriteArchiveSkillDocument(tempDir string, sourceURL string) (*adkskill.Frontmatter, string, error) {
	skillDoc, err := locateSkillDocument(tempDir)
	if err != nil {
		return nil, "", err
	}
	raw, err := os.ReadFile(skillDoc)
	if err != nil {
		return nil, "", err
	}
	if len(raw) > maxSkillFileSize {
		return nil, "", fmt.Errorf("skill file exceeds %d bytes", maxSkillFileSize)
	}
	fm, instructions, err := adkskill.ParseBytes(raw)
	if err != nil {
		return nil, "", err
	}
	if fm.Metadata == nil {
		fm.Metadata = map[string]string{}
	}
	fm.Metadata["source"] = sourceURL
	rebuilt, err := adkskill.Build(fm, instructions)
	if err != nil {
		return nil, "", err
	}
	if err := os.WriteFile(skillDoc, rebuilt, 0o644); err != nil {
		return nil, "", err
	}
	return fm, skillDoc, nil
}

func (r *SkillRegistry) loadInstalledArchiveSkill(ctx context.Context, name string) (Skill, error) {
	skill, ok, err := r.Get(ctx, name)
	if err != nil {
		return Skill{}, err
	}
	if !ok {
		return Skill{}, fmt.Errorf("installed skill not found: %s", name)
	}
	return skill, nil
}

func (r *SkillRegistry) Uninstall(ctx context.Context, id string) error {
	if r == nil || r.skillsPath == "" {
		return fmt.Errorf("skill registry is unavailable")
	}
	skill, ok, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return os.ErrNotExist
	}
	if strings.EqualFold(skill.Source, "builtin") {
		return fmt.Errorf("builtin skills cannot be uninstalled")
	}
	return os.RemoveAll(filepath.Join(r.skillsPath, strings.TrimSpace(id)))
}

func (r *SkillRegistry) ensureBuiltins() error {
	if err := r.removeLegacyBuiltinSkills(); err != nil {
		return err
	}
	for _, spec := range builtinSkillSpecs {
		bundle, err := spec.BuildBundle()
		if err != nil {
			return err
		}
		if err := r.syncBuiltinSkill(spec.Name, bundle); err != nil {
			return err
		}
	}
	return nil
}

func buildSingleFileBuiltinSkill(name string, description string, instructions string, allowedTools []string, version string) (map[string]string, error) {
	fm := &adkskill.Frontmatter{
		Name:         name,
		Description:  description,
		AllowedTools: allowedTools,
		Metadata: map[string]string{
			"source":  "builtin",
			"version": version,
		},
	}
	raw, err := adkskill.Build(fm, instructions)
	if err != nil {
		return nil, err
	}
	return map[string]string{"SKILL.md": string(raw)}, nil
}

func buildStrategyResearchBuiltinSkillBundle() (map[string]string, error) {
	bundle, err := buildSingleFileBuiltinSkill(
		strategypinespec.ResearchBuiltinSkillName,
		strategypinespec.ResearchSkillDescription(),
		strategypinespec.ResearchSkillInstructions(),
		strategypinespec.ResearchSkillAllowedTools(),
		strategypinespec.BuiltinSkillVersion,
	)
	if err != nil {
		return nil, err
	}
	maps.Copy(bundle, strategypinespec.ResearchSkillResourceFiles())
	return bundle, nil
}

func buildStrategyPublishBuiltinSkillBundle() (map[string]string, error) {
	bundle, err := buildSingleFileBuiltinSkill(
		strategypinespec.PublishBuiltinSkillName,
		strategypinespec.PublishSkillDescription(),
		strategypinespec.PublishSkillInstructions(),
		strategypinespec.PublishSkillAllowedTools(),
		strategypinespec.BuiltinSkillVersion,
	)
	if err != nil {
		return nil, err
	}
	maps.Copy(bundle, strategypinespec.PublishSkillResourceFiles())
	return bundle, nil
}

func (r *SkillRegistry) removeLegacyBuiltinSkills() error {
	for _, name := range []string{strategypinespec.LegacyBuiltinSkillName} {
		if err := r.removeLegacyBuiltinSkill(name); err != nil {
			return err
		}
	}
	return nil
}

func (r *SkillRegistry) removeLegacyBuiltinSkill(name string) error {
	if r == nil || r.skillsPath == "" {
		return nil
	}
	installDir := filepath.Join(r.skillsPath, strings.TrimSpace(name))
	skillDocPath := filepath.Join(installDir, "SKILL.md")
	raw, err := os.ReadFile(skillDocPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	fm, _, err := adkskill.ParseBytes(raw)
	if err != nil {
		return nil
	}
	if fm.Metadata != nil && strings.EqualFold(strings.TrimSpace(fm.Metadata["source"]), "builtin") {
		return os.RemoveAll(installDir)
	}
	return nil
}

func (r *SkillRegistry) syncBuiltinSkill(name string, bundle map[string]string) error {
	if r == nil || r.skillsPath == "" {
		return fmt.Errorf("skill registry is unavailable")
	}
	installDir := filepath.Join(r.skillsPath, name)
	skillDocPath := filepath.Join(installDir, "SKILL.md")
	raw, err := os.ReadFile(skillDocPath)
	switch {
	case os.IsNotExist(err):
		return replaceDirectoryWithBundle(installDir, bundle)
	case err != nil:
		return err
	}

	fm, _, err := adkskill.ParseBytes(raw)
	if err != nil {
		return err
	}
	if fm.Metadata != nil && !strings.EqualFold(strings.TrimSpace(fm.Metadata["source"]), "builtin") {
		return nil
	}
	if directoryMatchesBundle(installDir, bundle) {
		return nil
	}
	return replaceDirectoryWithBundle(installDir, bundle)
}

func (r *SkillRegistry) installSkillDocument(name string, raw []byte) (string, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false, fmt.Errorf("skill name is required")
	}
	installDir := filepath.Join(r.skillsPath, name)
	if _, err := os.Stat(installDir); err == nil {
		return "", true, fmt.Errorf("skill %q is already installed", name)
	} else if !os.IsNotExist(err) {
		return "", false, err
	}
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return "", false, err
	}
	installPath := filepath.Join(installDir, "SKILL.md")
	tempPath := installPath + ".tmp"
	if err := os.WriteFile(tempPath, raw, 0o644); err != nil {
		return "", false, err
	}
	if err := os.Rename(tempPath, installPath); err != nil {
		jftradeErr7 := os.Remove(tempPath)
		besteffort.LogError(jftradeErr7)
		jftradeErr10 := os.RemoveAll(installDir)
		besteffort.LogError(jftradeErr10)
		return "", false, err
	}
	return installPath, false, nil
}

func (r *SkillRegistry) installSkillDirectory(name string, sourceDir string) (string, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false, fmt.Errorf("skill name is required")
	}
	sourceDir = strings.TrimSpace(sourceDir)
	if sourceDir == "" {
		return "", false, fmt.Errorf("skill source directory is required")
	}
	installDir := filepath.Join(r.skillsPath, name)
	if _, err := os.Stat(installDir); err == nil {
		return "", true, fmt.Errorf("skill %q is already installed", name)
	} else if !os.IsNotExist(err) {
		return "", false, err
	}
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return "", false, err
	}
	if err := copyDirectoryContents(sourceDir, installDir); err != nil {
		jftradeErr9 := os.RemoveAll(installDir)
		besteffort.LogError(jftradeErr9)
		return "", false, err
	}
	return filepath.Join(installDir, "SKILL.md"), false, nil
}

func (r *SkillRegistry) source(ctx context.Context) (adkskill.Source, error) {
	if r == nil || r.skillsPath == "" {
		return nil, fmt.Errorf("skill registry is unavailable")
	}
	base := adkskill.NewFileSystemSource(os.DirFS(r.skillsPath))
	source, _, err := adkskill.WithCompletePreloadSource(ctx, base)
	if err != nil {
		return nil, err
	}
	return source, nil
}

func (r *SkillRegistry) skillFromFrontmatter(fm *adkskill.Frontmatter) (Skill, error) {
	installPath := filepath.Join(r.skillsPath, fm.Name, "SKILL.md")
	sourceName := "filesystem"
	version := ""
	if fm.Metadata != nil {
		if value := strings.TrimSpace(fm.Metadata["source"]); value != "" {
			sourceName = value
		}
		version = strings.TrimSpace(fm.Metadata["version"])
	}
	info, err := os.Stat(installPath)
	if err != nil {
		return Skill{}, err
	}
	raw, err := os.ReadFile(installPath)
	if err != nil {
		return Skill{}, err
	}
	hash := sha256.Sum256(raw)
	builtin := strings.EqualFold(sourceName, "builtin")
	return Skill{
		ID:               fm.Name,
		DisplayName:      fm.Name,
		Description:      fm.Description,
		Source:           sourceName,
		InstallPath:      installPath,
		Enabled:          true,
		Builtin:          builtin,
		Tools:            append([]string(nil), fm.AllowedTools...),
		Version:          version,
		ContentHash:      hex.EncodeToString(hash[:]),
		ValidationStatus: "VALID",
		CreatedAt:        info.ModTime().UTC().Format(time.RFC3339Nano),
		UpdatedAt:        info.ModTime().UTC().Format(time.RFC3339Nano),
	}, nil
}

type filteredSkillSource struct {
	base    adkskill.Source
	allowed map[string]struct{}
}

func (s *filteredSkillSource) ListFrontmatters(ctx context.Context) ([]*adkskill.Frontmatter, error) {
	items, err := s.base.ListFrontmatters(ctx)
	if err != nil {
		return nil, err
	}
	filtered := make([]*adkskill.Frontmatter, 0, len(items))
	for _, item := range items {
		if _, ok := s.allowed[item.Name]; ok {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func (s *filteredSkillSource) ListResources(ctx context.Context, name, subpath string) ([]string, error) {
	if err := s.allow(name); err != nil {
		return nil, err
	}
	return s.base.ListResources(ctx, name, subpath)
}

func (s *filteredSkillSource) LoadFrontmatter(ctx context.Context, name string) (*adkskill.Frontmatter, error) {
	if err := s.allow(name); err != nil {
		return nil, err
	}
	return s.base.LoadFrontmatter(ctx, name)
}

func (s *filteredSkillSource) LoadInstructions(ctx context.Context, name string) (string, error) {
	if err := s.allow(name); err != nil {
		return "", err
	}
	return s.base.LoadInstructions(ctx, name)
}

func (s *filteredSkillSource) LoadResource(ctx context.Context, name, resourcePath string) (io.ReadCloser, error) {
	if err := s.allow(name); err != nil {
		return nil, err
	}
	return s.base.LoadResource(ctx, name, resourcePath)
}

func (s *filteredSkillSource) allow(name string) error {
	if _, ok := s.allowed[strings.TrimSpace(name)]; !ok {
		return adkskill.ErrSkillNotFound
	}
	return nil
}

func sliceToSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		result[value] = struct{}{}
	}
	return result
}

func isZipSkillArchive(rawURL string, contentType string, body []byte) bool {
	lowerURL := strings.ToLower(strings.TrimSpace(rawURL))
	if strings.HasSuffix(lowerURL, ".zip") {
		return true
	}
	lowerType := strings.ToLower(strings.TrimSpace(contentType))
	if strings.Contains(lowerType, "zip") {
		return true
	}
	return len(body) >= 4 && bytes.Equal(body[:4], []byte{'P', 'K', 0x03, 0x04})
}

func locateSkillDocument(root string) (string, error) {
	matches := make([]string, 0, 1)
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "__MACOSX" {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() == "SKILL.md" {
			matches = append(matches, current)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("skill archive does not contain SKILL.md")
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("skill archive must contain exactly one SKILL.md")
	}
	return matches[0], nil
}

func copyDirectoryContents(sourceDir string, targetDir string) error {
	return filepath.WalkDir(sourceDir, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == sourceDir {
			return nil
		}
		relative, err := filepath.Rel(sourceDir, current)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(targetDir, relative)
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		input, err := os.Open(current)
		if err != nil {
			return err
		}
		output, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			jftradeErr3 := input.Close()
			besteffort.LogError(jftradeErr3)
			return err
		}
		if _, err := io.Copy(output, input); err != nil {
			jftradeErr4 := input.Close()
			besteffort.LogError(jftradeErr4)
			jftradeErr5 := output.Close()
			besteffort.LogError(jftradeErr5)
			return err
		}
		if err := input.Close(); err != nil {
			jftradeErr6 := output.Close()
			besteffort.LogError(jftradeErr6)
			return err
		}
		return output.Close()
	})
}

func directoryMatchesBundle(root string, bundle map[string]string) bool {
	expected := make(map[string]string, len(bundle))
	for relativePath, content := range bundle {
		expected[filepath.Clean(relativePath)] = content
	}

	actual := make(map[string]string, len(expected))
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == root || entry.IsDir() {
			return nil
		}
		relativePath, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		actual[filepath.Clean(relativePath)] = string(raw)
		return nil
	})
	if err != nil || len(actual) != len(expected) {
		return false
	}
	for relativePath, content := range expected {
		if actual[relativePath] != content {
			return false
		}
	}
	return true
}

func replaceDirectoryWithBundle(targetDir string, bundle map[string]string) error {
	parentDir := filepath.Dir(targetDir)
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp(parentDir, filepath.Base(targetDir)+".next-*")
	if err != nil {
		return err
	}
	defer func() { besteffort.LogError(os.RemoveAll(tempDir)) }()

	for relativePath, content := range bundle {
		cleanPath := filepath.Clean(relativePath)
		if cleanPath == "." || strings.HasPrefix(cleanPath, "..") {
			return fmt.Errorf("unsafe builtin skill bundle path %q", relativePath)
		}
		targetPath := filepath.Join(tempDir, cleanPath)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(targetPath, []byte(content), 0o644); err != nil {
			return err
		}
	}

	if _, err := os.Stat(targetDir); err != nil && !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		return os.Rename(tempDir, targetDir)
	}

	backupDir := targetDir + ".bak"
	jftradeErr8 := os.RemoveAll(backupDir)
	besteffort.LogError(jftradeErr8)
	if err := os.Rename(targetDir, backupDir); err != nil {
		return err
	}
	if err := os.Rename(tempDir, targetDir); err != nil {
		jftradeErr12 := os.Rename(backupDir, targetDir)
		besteffort.LogError(jftradeErr12)
		return err
	}
	return os.RemoveAll(backupDir)
}
