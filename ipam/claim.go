package ipam

import (
	"fmt"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/net/address"
	"github.com/weaveworks/weave/router"
)

type claim struct {
	resultChan chan<- error
	ident      string
	addr       address.Address
}

func (c *claim) sendResult(result error) {
	// Make sure we only send a result once, since listener stops listening after that
	if c.resultChan != nil {
		c.resultChan <- result
		close(c.resultChan)
		c.resultChan = nil
	}
	if result != nil {
		common.Log.Errorln("[allocator] " + result.Error())
	}
}

// Try returns true for success (or failure), false if we need to try again later
func (c *claim) Try(alloc *Allocator) bool {
	if !alloc.ring.Contains(c.addr) {
		// Address not within our universe; assume user knows what they are doing
		alloc.infof("Ignored address %s claimed by %s - not in our universe", c.addr, c.ident)
		c.sendResult(nil)
		return true
	}

	// If our ring doesn't know, it must be empty.  We will have initiated the
	// bootstrap of the ring, so wait until we find some owner for this
	// range (might be us).
	switch owner := alloc.ring.Owner(c.addr); owner {
	case alloc.ourName:
		// success
	case router.UnknownPeerName:
		alloc.infof("Ring is empty; will try later.", c.addr, owner)
		c.sendResult(nil) // don't make the caller wait
		return false
	default:
		name, found := alloc.nicknames[owner]
		if found {
			name = " (" + name + ")"
		}
		c.sendResult(fmt.Errorf("address %s is owned by other peer %s%s", c.addr.String(), owner, name))
		return true
	}

	// We are the owner, check we haven't given it to another container
	switch existingIdent := alloc.findOwner(c.addr); existingIdent {
	case "":
		if err := alloc.space.Claim(c.addr); err == nil {
			alloc.debugln("Claimed", c.addr, "for", c.ident)
			alloc.addOwned(c.ident, c.addr)
			c.sendResult(nil)
		} else {
			c.sendResult(err)
		}
	case c.ident:
		// same identifier is claiming same address; that's OK
		c.sendResult(nil)
	default:
		// Addr already owned by container on this machine
		c.sendResult(fmt.Errorf("address %s is already owned by %s", c.addr.String(), existingIdent))
	}
	return true
}

func (c *claim) Cancel() {
	c.sendResult(fmt.Errorf("Operation cancelled."))
}

func (c *claim) String() string {
	return fmt.Sprintf("Claim %s -> %s", c.ident, c.addr.String())
}

func (c *claim) ForContainer(ident string) bool {
	return c.ident == ident
}
