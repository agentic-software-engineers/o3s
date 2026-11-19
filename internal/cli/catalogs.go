package cli

import (
	"context"
	"fmt"
	"sync"

	"github.com/hansehart/o3s/internal/menu"
	"github.com/hansehart/o3s/internal/registry"
)

// catalogs reads the stores' catalogs in the background, once each for a run of the menu, so a view shows what is read
// while the rest comes in.
type catalogs struct {
	ctx  context.Context
	mu   sync.Mutex
	read map[string]*catalog
}

// catalog is one store's catalog: closed once it is read, with the features it offers or why it stayed out of reach.
type catalog struct {
	done   chan struct{}
	offers []menu.Offer
	err    error
}

// newCatalogs starts the catalogs of a run of the menu, read within its context.
func newCatalogs(ctx context.Context) *catalogs {
	return &catalogs{ctx: ctx, read: map[string]*catalog{}}
}

// fetch starts reading every store's catalog it has yet to read.
func (c *catalogs) fetch(stores []string) {
	// Start every store's read
	for _, s := range stores {
		c.of(s)
	}
}

// of hands back a store's catalog, starting its read the first time.
func (c *catalogs) of(store string) *catalog {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Take the catalog already reading or read
	if cat, ok := c.read[store]; ok {
		return cat
	}
	cat := &catalog{done: make(chan struct{})}
	c.read[store] = cat
	go func() {
		defer close(cat.done)
		features, err := registry.Collection(c.ctx, store)
		// Name the store whose features stay out of reach
		if err != nil {
			cat.err = fmt.Errorf("read the features of %s: %w", store, err)
			return
		}
		// Take every feature's id, name and description
		for _, f := range features {
			cat.offers = append(cat.offers, menu.Offer{ID: f.ID, Name: f.Name, Description: f.Description})
		}
	}()
	return cat
}

// wait hands back a store's catalog once it is read.
func (c *catalogs) wait(store string) ([]menu.Offer, error) {
	cat := c.of(store)
	<-cat.done
	return cat.offers, cat.err
}

// peek hands back a store's catalog where it is read already, and reports whether it is.
func (c *catalogs) peek(store string) ([]menu.Offer, bool, error) {
	cat := c.of(store)
	// Report a catalog still on its way
	select {
	case <-cat.done:
		return cat.offers, true, cat.err
	default:
		return nil, false, nil
	}
}
