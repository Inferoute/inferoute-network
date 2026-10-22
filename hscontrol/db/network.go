package db

import (
	"errors"
	"fmt"
	"strings"

	"github.com/juanfont/headscale/hscontrol/types"
	"gorm.io/gorm"
)

var (
	ErrNetworkNotFound  = errors.New("network not found")
	ErrNetworkNameTaken = errors.New("network name already exists")
	ErrNetworkIsDefault = errors.New("cannot delete the default network")
)

func seedDefaultNetwork(tx *gorm.DB) error {
	var count int64

	err := tx.Model(&types.Network{}).Where("name = ?", types.DefaultNetworkName).Count(&count).Error
	if err != nil {
		return fmt.Errorf("counting default network: %w", err)
	}

	if count > 0 {
		return nil
	}

	network := types.Network{Name: types.DefaultNetworkName}

	err = tx.Create(&network).Error
	if err != nil {
		return fmt.Errorf("seeding default network: %w", err)
	}

	return nil
}

// migrateNetworks creates the networks table, stamps network_id on existing
// rows, and scopes the user unique indexes. Fresh databases take InitSchema
// instead and skip this.
func migrateNetworks(tx *gorm.DB) error {
	if !tx.Migrator().HasTable(&types.Network{}) {
		err := tx.AutoMigrate(&types.Network{})
		if err != nil {
			return fmt.Errorf("creating networks table: %w", err)
		}
	}

	err := seedDefaultNetwork(tx)
	if err != nil {
		return err
	}

	var network types.Network

	err = tx.Where("name = ?", types.DefaultNetworkName).First(&network).Error
	if err != nil {
		return fmt.Errorf("reading default network: %w", err)
	}

	err = addNetworkColumn(tx, &types.User{}, "users", network.ID)
	if err != nil {
		return err
	}

	err = addNetworkColumn(tx, &types.Node{}, "nodes", network.ID)
	if err != nil {
		return err
	}

	err = addNetworkColumn(tx, &types.Policy{}, "policies", network.ID)
	if err != nil {
		return err
	}

	if !tx.Migrator().HasColumn(&types.Credential{}, "network_id") {
		err = tx.Migrator().AddColumn(&types.Credential{}, "NetworkID")
		if err != nil {
			return fmt.Errorf("adding credentials.network_id: %w", err)
		}
	}

	for _, stmt := range []string{
		`DROP INDEX IF EXISTS idx_provider_identifier`,
		`DROP INDEX IF EXISTS idx_name_provider_identifier`,
		`DROP INDEX IF EXISTS idx_name_no_provider_identifier`,
		`DROP INDEX IF EXISTS idx_networks_name`,
		`DROP INDEX IF EXISTS idx_networks_deleted_at`,
		`CREATE UNIQUE INDEX idx_provider_identifier ON users(network_id, provider_identifier) WHERE provider_identifier IS NOT NULL`,
		`CREATE UNIQUE INDEX idx_name_provider_identifier ON users(network_id, name, provider_identifier)`,
		`CREATE UNIQUE INDEX idx_name_no_provider_identifier ON users(network_id, name) WHERE provider_identifier IS NULL`,
		`CREATE UNIQUE INDEX idx_networks_name ON networks(name) WHERE deleted_at IS NULL`,
		`CREATE INDEX idx_networks_deleted_at ON networks(deleted_at)`,
	} {
		err = tx.Exec(stmt).Error
		if err != nil {
			return fmt.Errorf("network index %q: %w", stmt, err)
		}
	}

	return nil
}

func addNetworkColumn(tx *gorm.DB, model any, table string, networkID uint) error {
	if !tx.Migrator().HasColumn(model, "network_id") {
		err := tx.Migrator().AddColumn(model, "NetworkID")
		if err != nil {
			return fmt.Errorf("adding %s.network_id: %w", table, err)
		}
	}

	err := tx.Exec(
		"UPDATE "+table+" SET network_id = ? WHERE network_id IS NULL OR network_id = 0",
		networkID,
	).Error
	if err != nil {
		return fmt.Errorf("backfilling %s.network_id: %w", table, err)
	}

	return nil
}

type networkAndUser struct {
	Network *types.Network
	User    *types.User
}

func (hsdb *HSDatabase) CreateNetwork(name string) (*types.Network, *types.User, error) {
	created, err := Write(hsdb.DB, func(tx *gorm.DB) (*networkAndUser, error) {
		err := types.ValidateNetworkName(name)
		if err != nil {
			return nil, err
		}

		network := &types.Network{Name: name}

		err = tx.Create(network).Error
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				return nil, ErrNetworkNameTaken
			}

			return nil, err
		}

		user, err := CreateUser(tx, types.User{
			Name:      types.DefaultNetworkUser,
			NetworkID: network.ID,
		})
		if err != nil {
			return nil, err
		}

		return &networkAndUser{Network: network, User: user}, nil
	})
	if err != nil {
		return nil, nil, err
	}

	return created.Network, created.User, nil
}

func (hsdb *HSDatabase) GetNetwork(id uint) (*types.Network, error) {
	return Read(hsdb.DB, func(rx *gorm.DB) (*types.Network, error) {
		return getNetwork(rx, id)
	})
}

func (hsdb *HSDatabase) GetNetworkByName(name string) (*types.Network, error) {
	return Read(hsdb.DB, func(rx *gorm.DB) (*types.Network, error) {
		var network types.Network

		err := rx.Where("name = ?", name).First(&network).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrNetworkNotFound
			}

			return nil, err
		}

		return &network, nil
	})
}

func getNetwork(tx *gorm.DB, id uint) (*types.Network, error) {
	var network types.Network

	err := tx.First(&network, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNetworkNotFound
		}

		return nil, err
	}

	return &network, nil
}

func (hsdb *HSDatabase) ListNetworks() ([]types.Network, error) {
	return Read(hsdb.DB, func(rx *gorm.DB) ([]types.Network, error) {
		var networks []types.Network

		err := rx.Order("id").Find(&networks).Error

		return networks, err
	})
}

// GetOrCreateUserInNetwork returns the named user in one tailnet.
// The second result is true when this call inserted the row.
// The default user is not created here; creating a network already did that.
func (hsdb *HSDatabase) GetOrCreateUserInNetwork(networkID uint, name string) (*types.User, bool, error) {
	user, err := hsdb.GetUserInNetwork(networkID, name)
	if err == nil {
		return user, false, nil
	}

	if !errors.Is(err, ErrUserNotFound) || name == types.DefaultNetworkUser {
		return nil, false, err
	}

	created, err := hsdb.CreateUser(types.User{Name: name, NetworkID: networkID})
	if err != nil {
		return nil, false, err
	}

	return created, true, nil
}

func (hsdb *HSDatabase) GetUserInNetwork(networkID uint, name string) (*types.User, error) {
	return Read(hsdb.DB, func(rx *gorm.DB) (*types.User, error) {
		var user types.User

		err := rx.Where("network_id = ? AND name = ?", networkID, name).First(&user).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrUserNotFound
			}

			return nil, err
		}

		return &user, nil
	})
}

// DestroyNetwork removes an empty-of-nodes network and its users. Caller
// deletes nodes first. The default network is refused.
func (hsdb *HSDatabase) DestroyNetwork(id uint) error {
	return hsdb.Write(func(tx *gorm.DB) error {
		network, err := getNetwork(tx, id)
		if err != nil {
			return err
		}

		if network.Name == types.DefaultNetworkName {
			return ErrNetworkIsDefault
		}

		var users []types.User

		err = tx.Where("network_id = ?", id).Find(&users).Error
		if err != nil {
			return err
		}

		for _, user := range users {
			err = DestroyUser(tx, types.UserID(user.ID))
			if err != nil {
				return err
			}
		}

		err = tx.Where("network_id = ?", id).Delete(&types.Policy{}).Error
		if err != nil {
			return err
		}

		err = tx.Delete(network).Error
		if err != nil {
			return err
		}

		return nil
	})
}

func (hsdb *HSDatabase) ListNodeIDsByNetwork(networkID uint) ([]types.NodeID, error) {
	return Read(hsdb.DB, func(rx *gorm.DB) ([]types.NodeID, error) {
		var ids []types.NodeID

		err := rx.Model(&types.Node{}).Where("network_id = ?", networkID).Pluck("id", &ids).Error

		return ids, err
	})
}
