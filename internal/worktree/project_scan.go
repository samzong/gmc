package worktree

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func scanShareProjects(root string, rules []SharedResource) ([]DiscoverResult, []DiscoverResult, error) {
	var resources []DiscoverResult
	var buildDirectories []string
	manifests := make(map[string]map[string]bool)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		name := entry.Name()
		if name == ".git" || name == ".bare" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if _, err := os.Lstat(filepath.Join(path, ".git")); err == nil {
				return filepath.SkipDir
			}
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if shareRuleMatches(rules, rel) {
				resources = append(resources, DiscoverResult{Path: rel})
			}
			return nil
		}
		var candidate DiscoverResult
		candidate.Path = rel
		for _, suffix := range copyCandidates {
			if rel == suffix || strings.HasSuffix(rel, "/"+suffix) {
				candidate.Strategy, candidate.Reason = StrategyCopy, "config/env file; isolated copy per worktree"
			}
		}
		if entry.IsDir() {
			for _, suffix := range managedDirectories {
				if name == suffix {
					candidate.Status = "managed"
					candidate.Reason = "mutable dependency/cache directory; keep independent across worktrees; " +
						"use package-manager caches for reuse or configure an explicit share rule"
				}
			}
		}
		if _, covered := discoverCoveringRule(rules, rel); covered {
			if candidate.Strategy != "" || candidate.Status != "" || shareRuleMatches(rules, rel) {
				resources = append(resources, candidate)
			}
		} else if candidate.Strategy != "" || candidate.Status != "" {
			resources = append(resources, candidate)
		}
		if entry.IsDir() {
			if name == "target" && !shareRuleMatches(rules, rel) {
				buildDirectories = append(buildDirectories, rel)
			}
			switch name {
			case "node_modules", ".venv", "venv", "vendor", "__pycache__", "target", "build", "dist", ".next", ".nuxt",
				".local", ".cache", ".turbo", ".tox", ".mypy_cache", ".pytest_cache":
				return filepath.SkipDir
			}
			return nil
		}
		switch name {
		case "package.json", "pnpm-lock.yaml", "yarn.lock", "package-lock.json", "bun.lock", "bun.lockb",
			"pyproject.toml", "uv.lock", "requirements.txt", "Pipfile", "Pipfile.lock", "poetry.lock",
			"go.mod", "go.work", "Cargo.toml", "Cargo.lock":
			dir := filepath.ToSlash(filepath.Dir(rel))
			if manifests[dir] == nil {
				manifests[dir] = make(map[string]bool)
			}
			manifests[dir][name] = true
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	seen := make(map[string]bool)
	for _, resource := range resources {
		seen[resource.Path] = true
	}
	for _, rule := range rules {
		if seen[rule.Path] || strings.ContainsAny(rule.Path, "*?[") {
			continue
		}
		if err := ensureSharedDestination(root, rule.Path); err != nil {
			continue
		}
		if _, err := os.Lstat(filepath.Join(root, rule.Path, ".git")); err == nil {
			continue
		}
		insideRepository := false
		for parent := filepath.Dir(rule.Path); parent != "."; parent = filepath.Dir(parent) {
			if _, err := os.Lstat(filepath.Join(root, parent, ".git")); err == nil {
				insideRepository = true
				break
			}
		}
		if !insideRepository {
			if _, err := os.Lstat(filepath.Join(root, rule.Path)); err == nil {
				resources = append(resources, DiscoverResult{Path: rule.Path})
			}
		}
	}
	var projects []DiscoverResult
	for dir, files := range manifests {
		for _, project := range identifyShareProjects(files) {
			project.Project = dir
			project.Path = filepath.ToSlash(filepath.Join(dir, project.Path))
			project.Status = "managed"
			projects = append(projects, project)
		}
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].Path < projects[j].Path })
	for i, project := range projects {
		if project.Ecosystem != "Node.js" || filepath.Base(project.Path) != "package.json" {
			continue
		}
		var inherited DiscoverResult
		for _, parent := range projects {
			if parent.Ecosystem != "Node.js" || filepath.Base(parent.Path) == "package.json" {
				continue
			}
			if parent.Project == "." || strings.HasPrefix(project.Project, parent.Project+"/") {
				if len(parent.Project) > len(inherited.Project) {
					inherited = parent
				}
			}
		}
		if inherited.Project != "" {
			projects[i].PackageManager = inherited.PackageManager
			projects[i].Reason = inherited.Reason
		}
	}
	for _, path := range buildDirectories {
		if project, found := nearestShareProject(path, projects); found && project.Ecosystem == "Rust" {
			resources = append(resources, DiscoverResult{
				Path: path, Status: "managed",
				Reason: "Cargo build artifacts; keep target isolated instead of sharing a writable directory across branches",
			})
		}
	}
	return resources, projects, nil
}

func shareRuleMatches(rules []SharedResource, path string) bool {
	for _, rule := range rules {
		if MatchSharedPath(rule.Path, path) {
			return true
		}
	}
	return false
}

func identifyShareProjects(files map[string]bool) []DiscoverResult {
	var projects []DiscoverResult
	if files["package.json"] || files["pnpm-lock.yaml"] || files["yarn.lock"] ||
		files["package-lock.json"] || files["bun.lock"] || files["bun.lockb"] {
		p := DiscoverResult{
			Path: "package.json", Ecosystem: "Node.js", PackageManager: "npm",
			Reason: "package-manager caches can reuse downloaded packages; " +
				"keep node_modules independent across differing dependency graphs",
		}
		for _, choice := range []struct{ file, manager string }{
			{"package-lock.json", "npm"}, {"yarn.lock", "yarn"}, {"bun.lockb", "bun"},
			{"bun.lock", "bun"}, {"pnpm-lock.yaml", "pnpm"},
		} {
			if files[choice.file] {
				p.Path, p.PackageManager = choice.file, choice.manager
			}
		}
		if p.PackageManager == "pnpm" {
			p.Reason = "pnpm's content-addressable store can reuse package files; " +
				"directory sizes are not exclusive disk usage; keep branch dependency graphs independent"
		}
		projects = append(projects, p)
	}
	if files["pyproject.toml"] || files["uv.lock"] || files["requirements.txt"] ||
		files["Pipfile"] || files["poetry.lock"] || files["Pipfile.lock"] {
		p := DiscoverResult{
			Ecosystem: "Python", PackageManager: "pip",
			Reason: "package-manager caches reuse downloads and wheels; " +
				"virtual environments may embed absolute paths and should remain worktree-local",
		}
		for _, choice := range []struct{ file, manager string }{
			{"requirements.txt", "pip"}, {"pyproject.toml", "pip"}, {"Pipfile", "pipenv"},
			{"Pipfile.lock", "pipenv"}, {"poetry.lock", "poetry"}, {"uv.lock", "uv"},
		} {
			if files[choice.file] {
				p.Path, p.PackageManager = choice.file, choice.manager
			}
		}
		if p.PackageManager == "uv" {
			p.Reason = "uv's shared cache can reuse package files; " +
				"keep .venv local because environments can embed absolute paths"
		}
		projects = append(projects, p)
	}
	if files["go.mod"] || files["go.work"] {
		file := "go.mod"
		if files["go.work"] {
			file = "go.work"
		}
		projects = append(projects, DiscoverResult{
			Path: file, Ecosystem: "Go", PackageManager: "go",
			Reason: "Go already uses GOMODCACHE and GOCACHE outside the worktree by default; " +
				"inspect custom cache settings before adding share rules",
		})
	}
	if files["Cargo.toml"] || files["Cargo.lock"] {
		file := "Cargo.toml"
		if !files[file] {
			file = "Cargo.lock"
		}
		projects = append(projects, DiscoverResult{
			Path: file, Ecosystem: "Rust", PackageManager: "cargo",
			Reason: "Cargo reuses registry and Git downloads through CARGO_HOME; " +
				"target contains mutable build artifacts and is not recommended for blanket linking",
		})
	}
	return projects
}

func nearestShareProject(path string, projects []DiscoverResult) (DiscoverResult, bool) {
	wanted := ""
	switch filepath.Base(path) {
	case "node_modules":
		wanted = "Node.js"
	case ".venv", "venv", "__pycache__":
		wanted = "Python"
	case "target":
		wanted = "Rust"
	}
	var nearest DiscoverResult
	found := false
	for _, project := range projects {
		if wanted != "" && wanted != project.Ecosystem {
			continue
		}
		if project.Project == "." || strings.HasPrefix(path, project.Project+"/") {
			if !found || len(project.Project) > len(nearest.Project) {
				nearest, found = project, true
			}
		}
	}
	return nearest, found
}

func discoverLogicalSize(root string) (int64, bool) {
	var size int64
	limited := false
	count := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			limited = true
			return fs.SkipAll
		}
		count++
		if count > 20000 {
			limited = true
			return fs.SkipAll
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == ".bare" {
				return filepath.SkipDir
			}
			if path != root {
				if _, err := os.Lstat(filepath.Join(path, ".git")); err == nil {
					limited = true
					return filepath.SkipDir
				}
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			limited = true
			return fs.SkipAll
		}
		if info.Mode().IsRegular() {
			size += info.Size()
		}
		return nil
	})
	return size, limited || err != nil
}
