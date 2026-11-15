// Package project reads the repositories o3s project clones, and names the folders the cage keeps them in.
package project

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// Dir is where the cage keeps every project, the projects volume.
const Dir = "/home/ubuntu/projects"

// defaultHost is the host a repository names where it leaves the host out, as ghq and gh take it.
const defaultHost = "github.com"

// ErrInvalid marks a repository or project name o3s cannot read.
var ErrInvalid = errors.New("invalid")

// Repository is a repository o3s project clones.
type Repository struct {
	// Host is the host the clone connects to, and Port the port it opens there.
	Host string
	Port int
	// Owner is the path before the repository's name, its owner or group.
	Owner string
	// Name is the repository's name, which names its folder.
	Name string
	// URL is what git clones from.
	URL string
	// SSH marks a clone over SSH, which needs the SSH agent.
	SSH bool
}

// FullName names a repository as owner/name.
func (r Repository) FullName() string {
	return r.Owner + "/" + r.Name
}

// Path is the folder the cage keeps the repository in.
func (r Repository) Path() string {
	return Path(r.Name)
}

// Path is the folder the cage keeps a project of the given name in.
func Path(name string) string {
	return path.Join(Dir, name)
}

// scpLike matches git's short SSH form, user@host:path.
var scpLike = regexp.MustCompile(`^([A-Za-z0-9._-]+)@([A-Za-z0-9.-]+):(.+)$`)

// nameRule matches a folder name of one segment.
var nameRule = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Parse reads a repository in one of the forms ghq and gh take: an SSH or HTTPS URL, git's user@host:owner/repo, host/owner/repo, or owner/repo on github.com, the short forms cloned over SSH.
func Parse(arg string) (Repository, error) {
	r, err := parse(arg)
	// Name the forms o3s takes where the argument matches none
	if err != nil {
		return Repository{}, fmt.Errorf("%w repository %q, want owner/repo, host/owner/repo or a git URL: %w", ErrInvalid, arg, err)
	}
	return r, nil
}

// parse reads a repository and hands back why it fails to.
func parse(arg string) (Repository, error) {
	// Read git's short SSH form
	if m := scpLike.FindStringSubmatch(arg); m != nil && !strings.Contains(arg, "://") {
		return fromPath(m[2], 22, m[3], arg, true)
	}
	// Read a URL by its scheme
	if strings.Contains(arg, "://") {
		u, err := url.Parse(arg)
		// Hand back a URL that fails to read
		if err != nil {
			return Repository{}, err
		}
		port := map[string]int{"ssh": 22, "https": 443}[u.Scheme]
		// Take the two schemes the gateway lets through as a clone
		if port == 0 {
			return Repository{}, fmt.Errorf("scheme %q, want ssh or https", u.Scheme)
		}
		// Take the port the URL names
		if p := u.Port(); p != "" {
			port, err = strconv.Atoi(p)
			// Hand back a port that holds no number
			if err != nil {
				return Repository{}, err
			}
		}
		return fromPath(u.Hostname(), port, u.Path, arg, u.Scheme == "ssh")
	}
	parts := strings.Split(arg, "/")
	// Read owner/repo on the default host
	if len(parts) == 2 {
		return short(defaultHost, parts[0], parts[1])
	}
	// Read host/owner/repo
	if len(parts) == 3 && strings.Contains(parts[0], ".") {
		return short(parts[0], parts[1], parts[2])
	}
	return Repository{}, errors.New("no form matches")
}

// short builds the SSH clone of a repository named without a URL, the way gh clones with git_protocol ssh.
func short(host, owner, name string) (Repository, error) {
	name = strings.TrimSuffix(name, ".git")
	return fromPath(host, 22, owner+"/"+name, "git@"+host+":"+owner+"/"+name+".git", true)
}

// fromPath builds a repository from its host and the path that names its owner and name.
func fromPath(host string, port int, p, cloneURL string, ssh bool) (Repository, error) {
	p = strings.TrimSuffix(strings.Trim(p, "/"), ".git")
	owner, name := path.Split(p)
	owner = strings.Trim(owner, "/")
	// Turn away a path without an owner, or a name that makes no folder
	if host == "" || owner == "" || !ValidName(name) {
		return Repository{}, errors.New("want a host, an owner and a name")
	}
	return Repository{Host: host, Port: port, Owner: owner, Name: name, URL: cloneURL, SSH: ssh}, nil
}

// ValidName reports whether a name makes a folder of its own in the projects volume.
func ValidName(name string) bool {
	return nameRule.MatchString(name) && name != "." && name != ".."
}

// OwnerRepo reads owner/repo from a clone's remote URL, and reports whether the URL names one.
func OwnerRepo(remote string) (string, bool) {
	r, err := parse(strings.TrimSpace(remote))
	// Report a remote that names no repository
	if err != nil {
		return "", false
	}
	return r.FullName(), true
}
