package detect

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/b4b4r07/changed-objects/internal/git"
	"github.com/bmatcuk/doublestar/v4"
)

type client struct {
	args    []string
	opt     Option
	changes []git.Change
}

type Option struct {
	DefaultBranch string
	MergeBase     string
	Types         []string
	Ignores       []string
	GroupBy       string
	DirExist      string
}

func New(path string, args []string, opt Option) (client, error) {
	changes, err := git.Open(git.Config{
		Path:          path,
		DefaultBranch: opt.DefaultBranch,
		MergeBase:     opt.MergeBase,
	})
	if err != nil {
		return client{}, err
	}

	return client{
		args:    args,
		opt:     opt,
		changes: changes,
	}, nil
}

func (c client) Run() (Diff, error) {
	files, err := c.getFiles()
	if err != nil {
		return Diff{}, err
	}

	dirs, err := c.getDirs()
	if err != nil {
		return Diff{}, err
	}

	if len(c.opt.Types) > 0 {
		tmpFiles := Files{}
		for _, ty := range c.opt.Types {
			switch ty {
			case "added":
				tmpFiles = append(tmpFiles, files.filter(func(file File) bool {
					return file.Type == git.Addition
				})...)
			case "deleted":
				tmpFiles = append(tmpFiles, files.filter(func(file File) bool {
					return file.Type == git.Deletion
				})...)
			case "modified":
				tmpFiles = append(tmpFiles, files.filter(func(file File) bool {
					return file.Type == git.Modification
				})...)
			}
		}
		files = tmpFiles

		tmpDirs := Dirs{}
		for _, dir := range dirs {
			files := Files{}
			for _, ty := range c.opt.Types {
				switch ty {
				case "added":
					files = append(files, dir.Files.filter(func(file File) bool {
						return file.Type == git.Addition
					})...)
				case "deleted":
					files = append(files, dir.Files.filter(func(file File) bool {
						return file.Type == git.Deletion
					})...)
				case "modified":
					files = append(files, dir.Files.filter(func(file File) bool {
						return file.Type == git.Modification
					})...)
				}
			}
			if len(files) > 0 {
				dir.Files = files
				tmpDirs = append(tmpDirs, dir)
			}
		}
		dirs = tmpDirs
	}

	files = files.filter(func(file File) bool {
		switch c.opt.DirExist {
		case "true":
			return file.ParentDir.Exist
		case "false":
			return !file.ParentDir.Exist
		default:
			return true
		}
	})

	dirs = dirs.filter(func(dir Dir) bool {
		switch c.opt.DirExist {
		case "true":
			return dir.Exist
		case "false":
			return !dir.Exist
		default:
			return true
		}
	})

	return Diff{
		Files: files,
		Dirs:  dirs,
	}, nil
}

func (c client) getFiles() (Files, error) {
	var files Files

	for _, change := range c.changes {
		if len(c.opt.GroupBy) > 0 {
			matched, _ := doublestar.Match(filepath.Join(c.opt.GroupBy, "**"), change.Path)
			if !matched {
				log.Printf("[DEBUG] getFiles: %s is not matched in %s\n", change.Path, c.opt.GroupBy)
				continue
			}
		}
		files = append(files, getFile(change))
	}

	for _, arg := range c.args {
		files = files.filter(func(file File) bool {
			return strings.Index(file.Path, arg) == 0
		})
	}

	for _, ignore := range c.opt.Ignores {
		files = files.filter(func(file File) bool {
			match, err := doublestar.Match(ignore, file.Path)
			if err != nil {
				return false
			}
			return !match
		})
	}

	return files, nil
}

func (c client) getDirs() (Dirs, error) {
	matrix := make(map[string]Dir)

	for _, change := range c.changes {
		path := change.Dir
		if len(c.opt.GroupBy) > 0 {
			length := len(strings.Split(c.opt.GroupBy, "/"))
			matched, _ := doublestar.Match(filepath.Join(c.opt.GroupBy, "**"), change.Path)
			if !matched {
				log.Printf("[DEBUG] getDirs: %s is not matched in %s\n", change.Path, c.opt.GroupBy)
				continue
			}
			path = strings.Join(strings.Split(change.Path, "/")[0:length], "/")
			log.Printf("[DEBUG] getDirs: chunk path %s\n", path)
		}
		dir, ok := matrix[path]
		if ok {
			log.Printf("[TRACE] getDirs: updated %q", path)
			dir.Files = append(dir.Files, getFile(change))
		} else {
			log.Printf("[TRACE] getDirs: created %q", path)
			dir = Dir{
				Path: path,
				Exist: func() bool {
					_, err := os.Stat(path)
					return err == nil
				}(),
				Files: Files{getFile(change)},
			}
		}
		log.Printf("[TRACE] getDirs: add %q to dir matrix", change.Path)
		matrix[path] = dir
	}

	var dirs Dirs
	for _, dir := range matrix {
		log.Printf("[TRACE] getDirs: convert dirs matrix to slice: %q", dir.Path)
		dirs = append(dirs, dir)
	}

	for _, arg := range c.args {
		dirs = dirs.filter(func(dir Dir) bool {
			return strings.Index(dir.Path, arg) == 0
		})
	}

	for _, ignore := range c.opt.Ignores {
		dirs = dirs.filter(func(dir Dir) bool {
			match, err := doublestar.Match(ignore, dir.Path)
			if err != nil {
				return false
			}
			return !match
		})
	}

	return dirs, nil
}
