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

// defaultHost is the host a repository names where it leaves the host out.
const defaultHost = "github.com"

// ErrInvalid marks a repository or project name o3s fails to read.
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

// Short names a repository as host/owner/name, the form Parse reads back.
func (r Repository) Short() string {
	return r.Host + "/" + r.FullName()
}

// Path is the folder the cage keeps the repository in, by host, owner and name.
func (r Repository) Path() string {
	return path.Join(Dir, r.Short())
}

// At reads the repository a folder of the projects volume holds, and reports whether the folder is one.
func At(folder string) (Repository, bool) {
	rest, ok := strings.CutPrefix(folder, Dir+"/")
	// Report a folder outside the projects volume
	if !ok {
		return Repository{}, false
	}
	r, err := parse(rest)
	return r, err == nil && r.Path() == folder
}

// scpLike matches git's short SSH form, user@host:path.
var scpLike = regexp.MustCompile(`^([A-Za-z0-9._-]+)@([A-Za-z0-9.-]+):(.+)$`)

// segment matches one folder of a project's path.
var segment = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Parse reads a repository given as a URL, in git's short SSH form, or as host/owner/name or owner/name on the default host, the short forms cloned over SSH.
func Parse(arg string) (Repository, error) {
	r, err := parse(arg)
	// Name the forms o3s takes when the argument fails to match them
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
		// Take the two schemes the allowlist lets through as a clone
		if port == 0 {
			return Repository{}, fmt.Errorf("scheme %q, want ssh or https", u.Scheme)
		}
		// Take the port the URL names
		if p := u.Port(); p != "" {
			port, err = strconv.Atoi(p)
			// Hand back a port that fails to read as a number
			if err != nil {
				return Repository{}, err
			}
		}
		return fromPath(u.Hostname(), port, u.Path, arg, u.Scheme == "ssh")
	}
	parts := strings.Split(arg, "/")
	// Read owner/repo on the default host
	if len(parts) == 2 {
		return short(defaultHost, arg)
	}
	// Read host/owner/repo, the owner with any subgroups
	if len(parts) >= 3 && strings.Contains(parts[0], ".") {
		return short(parts[0], strings.Join(parts[1:], "/"))
	}
	return Repository{}, errors.New("no form matches")
}

// short builds the SSH clone of a repository named in short form.
func short(host, p string) (Repository, error) {
	p = strings.TrimSuffix(p, ".git")
	return fromPath(host, 22, p, "git@"+host+":"+p+".git", true)
}

// fromPath builds a repository from its host and the path that names its owner and name.
func fromPath(host string, port int, p, cloneURL string, ssh bool) (Repository, error) {
	p = strings.TrimSuffix(strings.Trim(p, "/"), ".git")
	parts := append([]string{host}, strings.Split(p, "/")...)
	// Turn away a path that lacks an owner
	if len(parts) < 3 {
		return Repository{}, errors.New("want a host, an owner and a name")
	}
	// Turn away any part that fails to make a folder of its own in the projects volume
	for _, part := range parts {
		// Name the part that would leave its place
		if !segment.MatchString(part) || strings.Trim(part, ".") == "" {
			return Repository{}, fmt.Errorf("%q makes no folder", part)
		}
	}
	n := len(parts) - 1
	return Repository{Host: host, Port: port, Owner: strings.Join(parts[1:n], "/"), Name: parts[n], URL: cloneURL, SSH: ssh}, nil
}
