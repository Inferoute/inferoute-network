package types

import (
	"errors"

	"gorm.io/gorm"
)

var (
	ErrPolicyNotFound         = errors.New("acl policy not found")
	ErrPolicyUpdateIsDisabled = errors.New("update is disabled for modes other than 'database'")
)

// Policy represents a policy in the database.
type Policy struct {
	gorm.Model

	// Data contains the policy in HuJSON format.
	Data string

	// NetworkID scopes the policy row. Zero is rewritten to the default
	// network on insert. Selection of per-network policy is not wired yet:
	// the running policy manager still loads the latest row.
	NetworkID uint `json:"-" gorm:"not null;default:1"`
}
