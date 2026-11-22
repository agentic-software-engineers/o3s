// Package registry reads the feature stores o3s offers features from.
package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// collection is the media type the Dev Container spec gives a store's collection.
const collection types.MediaType = "application/vnd.devcontainers.collection.layer.v1+json"

// ErrNoCollection marks an artifact that lacks a feature collection.
var ErrNoCollection = errors.New("holds no feature collection")

// ErrDenied marks a store its registry serves to logged-in users alone.
var ErrDenied = errors.New("denies access")

// ErrOutOfReach marks a store whose registry fails to answer.
var ErrOutOfReach = errors.New("is out of reach")

// Feature is what a store tells about one of its features.
type Feature struct {
	ID      string
	Version string
	// Name and Description are how the store names the feature and says what it does.
	Name, Description string
	Options           []Option
	Hosts             []Host
	Env               []Env
	// After are the features this one installs after, as store/id: the ones it depends on and the ones it
	// names to follow.
	After []string
}

// Option is a setting a feature offers, with the values it takes and the one it falls back to.
type Option struct {
	Name string
	// Type is boolean or string.
	Type    string
	Default string
	// Enum lists the full set of values the option takes, and Proposals the ones it suggests while it takes others too.
	Enum      []string
	Proposals []string
	// Description says what the option does.
	Description string
}

// optionSpec is an option as a feature's metadata writes it.
type optionSpec struct {
	Type        string   `json:"type"`
	Default     any      `json:"default"`
	Enum        []string `json:"enum"`
	Proposals   []string `json:"proposals"`
	Description string   `json:"description"`
}

// Host is a host a feature needs, with the secret the proxy injects there.
type Host struct {
	Name   string   `json:"name"`
	Ports  []int    `json:"ports"`
	Secret string   `json:"secret"`
	Inject []string `json:"inject"`
}

// Env is a variable a feature sets for every process in the cage.
type Env struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Collection fetches a store's collection with the user's registry logins and hands back its features in id order.
func Collection(ctx context.Context, store string) ([]Feature, error) {
	ref, err := name.ParseReference(store + ":latest")
	// Hand back a store name that fails to parse
	if err != nil {
		return nil, err
	}
	img, err := remote.Image(ref, remote.WithContext(ctx), remote.WithAuthFromKeychain(authn.DefaultKeychain))
	// Hand back why the registry fails to serve the store
	if err != nil {
		return nil, served(ref.Context().RegistryStr(), err)
	}
	layers, err := img.Layers()
	// Hand back an artifact whose layers fail to read
	if err != nil {
		return nil, err
	}
	// Read the first layer that holds the collection
	for _, l := range layers {
		mt, err := l.MediaType()
		// Hand back a layer whose type fails to read
		if err != nil {
			return nil, err
		}
		// Look further while the layer holds something else
		if mt != collection {
			continue
		}
		return features(l)
	}
	return nil, fmt.Errorf("%s %w", store, ErrNoCollection)
}

// features reads the features a collection layer lists, in id order.
func features(l v1.Layer) ([]Feature, error) {
	rc, err := l.Compressed()
	// Hand back a layer that fails to open
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	var c struct {
		Features []struct {
			ID             string                `json:"id"`
			Version        string                `json:"version"`
			Name           string                `json:"name"`
			Description    string                `json:"description"`
			Options        map[string]optionSpec `json:"options"`
			InstallsAfter  []string              `json:"installsAfter"`
			DependsOn      map[string]any        `json:"dependsOn"`
			Customizations struct {
				O3S struct {
					Hosts []Host `json:"hosts"`
					Env   []Env  `json:"env"`
				} `json:"o3s"`
			} `json:"customizations"`
		} `json:"features"`
	}
	// Hand back a collection that fails to decode
	if err := json.NewDecoder(rc).Decode(&c); err != nil {
		return nil, fmt.Errorf("read the collection: %w", err)
	}
	fs := make([]Feature, len(c.Features))
	// Take what every feature tells
	for i, f := range c.Features {
		o := f.Customizations.O3S
		fs[i] = Feature{ID: f.ID, Version: f.Version, Name: f.Name, Description: f.Description, Hosts: o.Hosts, Env: o.Env}
		// Take every feature this one names to follow, stripped of its tag
		for _, id := range append(slices.Collect(maps.Keys(f.DependsOn)), f.InstallsAfter...) {
			// Take each feature once
			if pkg := packageOf(id); !slices.Contains(fs[i].After, pkg) {
				fs[i].After = append(fs[i].After, pkg)
			}
		}
		slices.Sort(fs[i].After)
		// List the options the feature offers, with what each takes
		for name, o := range f.Options {
			fs[i].Options = append(fs[i].Options, Option{Name: name, Type: o.Type, Default: defaultOf(o.Default), Enum: o.Enum, Proposals: o.Proposals, Description: o.Description})
		}
		slices.SortFunc(fs[i].Options, func(a, b Option) int { return strings.Compare(a.Name, b.Name) })
	}
	slices.SortFunc(fs, func(a, b Feature) int { return strings.Compare(a.ID, b.ID) })
	return fs, nil
}

// packageOf strips the tag or the digest off a feature's id, leaving store/id.
func packageOf(id string) string {
	slash := strings.LastIndex(id, "/")
	// Cut at the digest or the tag that follows the last part of the path
	if i := strings.IndexAny(id[slash+1:], "@:"); i >= 0 {
		return id[:slash+1+i]
	}
	return id
}

// defaultOf writes an option's fallback as text, a boolean as true or false.
func defaultOf(v any) string {
	// Write the fallback in the shape it came in
	switch d := v.(type) {
	case nil:
		return ""
	case string:
		return d
	default:
		return fmt.Sprint(d)
	}
}

// served names why a registry turns a store away, by what it answered, keeping the addresses it was asked at out of it:
// a login it wants, a store it lacks, or an answer it fails to give.
func served(host string, err error) error {
	var answer *transport.Error
	var asked *url.Error
	// Name the cause by the registry's answer, or by the request that went unanswered
	switch {
	case errors.As(err, &answer) && (answer.StatusCode == http.StatusUnauthorized || answer.StatusCode == http.StatusForbidden):
		return fmt.Errorf("%w, log in with docker login %s", ErrDenied, host)
	case errors.As(err, &answer) && answer.StatusCode == http.StatusNotFound:
		return ErrNoCollection
	case errors.As(err, &answer):
		return fmt.Errorf("%w (%s)", ErrOutOfReach, http.StatusText(answer.StatusCode))
	case errors.As(err, &asked):
		return fmt.Errorf("%w (%w)", ErrOutOfReach, asked.Err)
	}
	return err
}
