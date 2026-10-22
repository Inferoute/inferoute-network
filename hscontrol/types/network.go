package types

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const (
	// DefaultNetworkName is the tailnet implicit callers land in when they
	// do not pass a network. It is seeded on migrate and first boot.
	DefaultNetworkName = "default"

	// DefaultNetworkUser is the single user created with a network. Cluster
	// members are nodes of this user, not separate users.
	DefaultNetworkUser = "default"
)

var ErrInvalidNetworkName = errors.New("invalid network name")

// Network is one tailnet. Nodes in different networks never share a peer map.
type Network struct {
	gorm.Model

	Name string `gorm:"not null"`
}

// ValidateNetworkName rejects empty names and whitespace. Cluster ids from
// Inferoute are opaque strings, so this is intentionally looser than a username.
func ValidateNetworkName(name string) error {
	if name == "" || len(name) > 63 || strings.ContainsAny(name, " \t\r\n/") {
		return fmt.Errorf("%w: %q", ErrInvalidNetworkName, name)
	}

	return nil
}

func (v NodeView) NetworkID() uint {
	if !v.Valid() {
		return 0
	}

	return v.ж.NetworkID
}

func (v UserView) NetworkID() uint {
	if !v.Valid() {
		return 0
	}

	return v.ж.NetworkID
}

func assignDefaultNetwork(tx *gorm.DB, id *uint) error {
	if id == nil || *id != 0 {
		return nil
	}

	var network Network

	err := tx.Where("name = ?", DefaultNetworkName).First(&network).Error
	if err != nil {
		return fmt.Errorf("default network: %w", err)
	}

	*id = network.ID

	return nil
}

func (u *User) BeforeCreate(tx *gorm.DB) error {
	return assignDefaultNetwork(tx, &u.NetworkID)
}

func (n *Node) BeforeSave(tx *gorm.DB) error {
	if n.NetworkID != 0 {
		return nil
	}

	if n.User != nil && n.User.NetworkID != 0 {
		n.NetworkID = n.User.NetworkID

		return nil
	}

	if n.UserID != nil {
		var networkID uint

		err := tx.Model(&User{}).Where("id = ?", *n.UserID).Pluck("network_id", &networkID).Error
		if err != nil {
			return fmt.Errorf("node network from user: %w", err)
		}

		if networkID != 0 {
			n.NetworkID = networkID

			return nil
		}
	}

	return assignDefaultNetwork(tx, &n.NetworkID)
}

func (p *Policy) BeforeCreate(tx *gorm.DB) error {
	return assignDefaultNetwork(tx, &p.NetworkID)
}

func (c *Credential) BeforeCreate(tx *gorm.DB) error {
	if c.NetworkID != nil || c.Kind != CredentialPreAuthKey || c.UserID == nil {
		return nil
	}

	var networkID uint

	err := tx.Model(&User{}).Where("id = ?", *c.UserID).Pluck("network_id", &networkID).Error
	if err != nil {
		return fmt.Errorf("preauth key network: %w", err)
	}

	if networkID == 0 {
		return nil
	}

	c.NetworkID = &networkID

	return nil
}
