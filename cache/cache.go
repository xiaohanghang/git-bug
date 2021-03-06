package cache

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strconv"

	"github.com/MichaelMure/git-bug/repository"
	"github.com/MichaelMure/git-bug/util"
)

const lockfile = "lock"
const excerptsFile = "excerpts"

type RootCache struct {
	repos map[string]*RepoCache
}

func NewCache() RootCache {
	return RootCache{
		repos: make(map[string]*RepoCache),
	}
}

// RegisterRepository register a named repository. Use this for multi-repo setup
func (c *RootCache) RegisterRepository(ref string, repo repository.Repo) error {
	err := c.lockRepository(repo)
	if err != nil {
		return err
	}

	r, err := NewRepoCache(repo)
	if err != nil {
		return err
	}

	c.repos[ref] = r
	return nil
}

// RegisterDefaultRepository register a unnamed repository. Use this for mono-repo setup
func (c *RootCache) RegisterDefaultRepository(repo repository.Repo) error {
	err := c.lockRepository(repo)
	if err != nil {
		return err
	}

	r, err := NewRepoCache(repo)
	if err != nil {
		return err
	}

	c.repos[""] = r
	return nil
}

func (c *RootCache) lockRepository(repo repository.Repo) error {
	lockPath := repoLockFilePath(repo)

	err := RepoIsAvailable(repo)
	if err != nil {
		return err
	}

	f, err := os.Create(lockPath)
	if err != nil {
		return err
	}

	pid := fmt.Sprintf("%d", os.Getpid())
	_, err = f.WriteString(pid)
	if err != nil {
		return err
	}

	return f.Close()
}

// ResolveRepo retrieve a repository by name
func (c *RootCache) DefaultRepo() (*RepoCache, error) {
	if len(c.repos) != 1 {
		return nil, fmt.Errorf("repository is not unique")
	}

	for _, r := range c.repos {
		return r, nil
	}

	panic("unreachable")
}

// DefaultRepo retrieve the default repository
func (c *RootCache) ResolveRepo(ref string) (*RepoCache, error) {
	r, ok := c.repos[ref]
	if !ok {
		return nil, fmt.Errorf("unknown repo")
	}
	return r, nil
}

// Close will do anything that is needed to close the cache properly
func (c *RootCache) Close() error {
	for _, cachedRepo := range c.repos {
		lockPath := repoLockFilePath(cachedRepo.repo)
		err := os.Remove(lockPath)
		if err != nil {
			return err
		}
	}
	return nil
}

// RepoIsAvailable check is the given repository is locked by a Cache.
// Note: this is a smart function that will cleanup the lock file if the
// corresponding process is not there anymore.
// If no error is returned, the repo is free to edit.
func RepoIsAvailable(repo repository.Repo) error {
	lockPath := repoLockFilePath(repo)

	// Todo: this leave way for a racey access to the repo between the test
	// if the file exist and the actual write. It's probably not a problem in
	// practice because using a repository will be done from user interaction
	// or in a context where a single instance of git-bug is already guaranteed
	// (say, a server with the web UI running). But still, that might be nice to
	// have a mutex or something to guard that.

	// Todo: this will fail if somehow the filesystem is shared with another
	// computer. Should add a configuration that prevent the cleaning of the
	// lock file

	f, err := os.Open(lockPath)

	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if err == nil {
		// lock file already exist
		buf, err := ioutil.ReadAll(io.LimitReader(f, 10))
		if err != nil {
			return err
		}
		if len(buf) == 10 {
			return fmt.Errorf("The lock file should be < 10 bytes")
		}

		pid, err := strconv.Atoi(string(buf))
		if err != nil {
			return err
		}

		if util.ProcessIsRunning(pid) {
			return fmt.Errorf("The repository you want to access is already locked by the process pid %d", pid)
		}

		// The lock file is just laying there after a crash, clean it

		fmt.Println("A lock file is present but the corresponding process is not, removing it.")
		err = f.Close()
		if err != nil {
			return err
		}

		os.Remove(lockPath)
		if err != nil {
			return err
		}
	}

	return nil
}

func repoLockFilePath(repo repository.Repo) string {
	return path.Join(repo.GetPath(), ".git", "git-bug", lockfile)
}
