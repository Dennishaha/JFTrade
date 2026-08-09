package adk

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

	adkskill "google.golang.org/adk/v2/tool/skilltoolset/skill"

	"github.com/jftrade/jftrade-main/internal/assistant/engine/providers"
	"github.com/jftrade/jftrade-main/internal/assistant/engine/skillsruntime"
)

const WorkflowManagementSkillName = skillsruntime.WorkflowManagementSkillName

type filteredSkillSource = skillsruntime.FilteredSkillSource

// WorkflowManagementToolNames returns the tools unlocked by the builtin
// workflow-management skill.
func WorkflowManagementToolNames() []string {
	return skillsruntime.WorkflowManagementToolNames()
}

func BuiltinSkillIDs() []string {
	return skillsruntime.BuiltinSkillIDs()
}

// SkillRegistry is the runner-facing skill installation and lookup facade. The
// filesystem registry, builtin catalog and install pipeline live in
// skillsruntime; this type keeps the public engine surface and test behavior.
type SkillRegistry struct {
	skillsPath string
	impl       *skillsruntime.SkillRegistry
}

func NewSkillRegistry(skillsPath string) *SkillRegistry {
	path := strings.TrimSpace(skillsPath)
	if path == "" {
		return &SkillRegistry{}
	}
	return &SkillRegistry{skillsPath: path, impl: skillsruntime.NewSkillRegistry(path)}
}

func (r *SkillRegistry) registry() *skillsruntime.SkillRegistry {
	if r == nil {
		return nil
	}
	if r.impl == nil && strings.TrimSpace(r.skillsPath) != "" {
		r.impl = skillsruntime.NewSkillRegistryFromPath(r.skillsPath)
	}
	return r.impl
}

func (r *SkillRegistry) List(ctx context.Context) ([]Skill, error) {
	return r.registry().List(ctx)
}

func (r *SkillRegistry) Get(ctx context.Context, id string) (Skill, bool, error) {
	return r.registry().Get(ctx, id)
}

func (r *SkillRegistry) Source(ctx context.Context, names []string) (adkskill.Source, error) {
	if r != nil && strings.TrimSpace(r.skillsPath) == "" && r.impl == nil {
		return nil, fmt.Errorf("skill registry is unavailable")
	}
	return r.registry().Source(ctx, names)
}

func (r *SkillRegistry) InstallURL(ctx context.Context, rawURL string) (Skill, error) {
	return r.registry().InstallURL(ctx, rawURL)
}

func (r *SkillRegistry) Uninstall(ctx context.Context, id string) error {
	return r.registry().Uninstall(ctx, id)
}

func (r *SkillRegistry) installArchive(ctx context.Context, sourceURL string, body []byte) (Skill, error) {
	return r.registry().InstallArchive(ctx, sourceURL, body)
}

func (r *SkillRegistry) installExtractedArchiveSkill(ctx context.Context, sourceURL string, tempDir string) (Skill, error) {
	return r.registry().InstallExtractedArchiveSkill(ctx, sourceURL, tempDir)
}

func (r *SkillRegistry) ensureBuiltins() error {
	return r.registry().EnsureBuiltins()
}

func (r *SkillRegistry) syncBuiltinSkill(name string, bundle map[string]string) error {
	return r.registry().SyncBuiltinSkill(name, bundle)
}

func (r *SkillRegistry) installSkillDocument(name string, raw []byte) (string, bool, error) {
	return r.registry().InstallSkillDocument(name, raw)
}

func (r *SkillRegistry) installSkillDirectory(name string, sourceDir string) (string, bool, error) {
	return r.registry().InstallSkillDirectory(name, sourceDir)
}

func (r *SkillRegistry) source(ctx context.Context) (adkskill.Source, error) {
	return r.registry().FileSource(ctx)
}

func (r *SkillRegistry) skillFromFrontmatter(fm *adkskill.Frontmatter) (Skill, error) {
	return r.registry().SkillFromFrontmatter(fm)
}

func builtinSkillMetadataCatalog() ([]Skill, error) {
	return skillsruntime.BuiltinSkillMetadataCatalog()
}

func builtinSkillAllowsAuthorizedToolSubset(name string) bool {
	return skillsruntime.BuiltinSkillAllowsAuthorizedToolSubset(name)
}

func buildSingleFileBuiltinSkill(name string, description string, instructions string, allowedTools []string, version string) (map[string]string, error) {
	return skillsruntime.BuildSingleFileBuiltinSkill(name, description, instructions, allowedTools, version)
}

func isZipSkillArchive(rawURL string, contentType string, body []byte) bool {
	return skillsruntime.IsZipSkillArchive(rawURL, contentType, body)
}

func locateSkillDocument(root string) (string, error) {
	return skillsruntime.LocateSkillDocument(root)
}

func copyDirectoryContents(sourceDir string, targetDir string) error {
	return skillsruntime.CopyDirectoryContents(sourceDir, targetDir)
}

func directoryMatchesBundle(root string, bundle map[string]string) bool {
	return skillsruntime.DirectoryMatchesBundle(root, bundle)
}

func replaceDirectoryWithBundle(targetDir string, bundle map[string]string) error {
	return skillsruntime.ReplaceDirectoryWithBundle(targetDir, bundle)
}

func sliceToSet(values []string) map[string]struct{} {
	return skillsruntime.SliceToSet(values)
}

func unsafeAddr(addr netip.Addr) bool {
	return providers.IsUnsafeAddr(addr)
}
