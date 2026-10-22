package apiv1

import (
	"context"
	"net/http"
	"strconv"

	"github.com/danielgtaylor/huma/v2"
	"github.com/juanfont/headscale/hscontrol/types"
)

func init() {
	registrations = append(registrations, registerNetworks)
}

// Network is one tailnet managed by this process.
type Network struct {
	ID   string `format:"uint64" json:"id"`
	Name string `json:"name"`
}

type (
	createNetworkInput struct {
		Body struct {
			Name string `json:"name"`
		}
	}
	networkOutput struct {
		Body struct {
			Network Network `json:"network"`
		}
	}
	listNetworksOutput struct {
		Body struct {
			Networks []Network `json:"networks" nullable:"false"`
		}
	}
	getNetworkInput struct {
		ID string `format:"uint64" path:"id"`
	}
	deleteNetworkInput struct {
		ID string `format:"uint64" path:"id"`
	}
	deleteNetworkOutput struct {
		Body struct{}
	}
)

func registerNetworks(api huma.API, b Backend) {
	huma.Register(api, huma.Operation{
		OperationID: "createNetwork",
		Method:      http.MethodPost,
		Path:        "/api/v1/network",
		Summary:     "Create network",
		Tags:        []string{"Networks"},
		Security:    bearerAuth,
	}, func(ctx context.Context, in *createNetworkInput) (*networkOutput, error) {
		network, _, c, err := b.State.CreateNetwork(in.Body.Name)
		if err != nil {
			return nil, mapError("creating network", err)
		}

		b.Change(c)

		out := &networkOutput{}
		out.Body.Network = networkFromModel(network)

		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "listNetworks",
		Method:      http.MethodGet,
		Path:        "/api/v1/network",
		Summary:     "List networks",
		Tags:        []string{"Networks"},
		Security:    bearerAuth,
	}, func(ctx context.Context, in *struct{}) (*listNetworksOutput, error) {
		networks, err := b.State.ListNetworks()
		if err != nil {
			return nil, mapError("listing networks", err)
		}

		out := &listNetworksOutput{}
		out.Body.Networks = make([]Network, 0, len(networks))

		for i := range networks {
			out.Body.Networks = append(out.Body.Networks, networkFromModel(&networks[i]))
		}

		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "getNetwork",
		Method:      http.MethodGet,
		Path:        "/api/v1/network/{id}",
		Summary:     "Get network",
		Tags:        []string{"Networks"},
		Security:    bearerAuth,
	}, func(ctx context.Context, in *getNetworkInput) (*networkOutput, error) {
		id, err := parseNetworkID(in.ID)
		if err != nil {
			return nil, err
		}

		network, err := b.State.GetNetwork(id)
		if err != nil {
			return nil, mapError("getting network", err)
		}

		out := &networkOutput{}
		out.Body.Network = networkFromModel(network)

		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "deleteNetwork",
		Method:      http.MethodDelete,
		Path:        "/api/v1/network/{id}",
		Summary:     "Delete network",
		Tags:        []string{"Networks"},
		Security:    bearerAuth,
	}, func(ctx context.Context, in *deleteNetworkInput) (*deleteNetworkOutput, error) {
		id, err := parseNetworkID(in.ID)
		if err != nil {
			return nil, err
		}

		changes, err := b.State.DeleteNetwork(id)
		if err != nil {
			return nil, mapError("deleting network", err)
		}

		b.Change(changes...)

		return &deleteNetworkOutput{}, nil
	})
}

func networkFromModel(network *types.Network) Network {
	return Network{
		ID:   strconv.FormatUint(uint64(network.ID), 10),
		Name: network.Name,
	}
}

func parseNetworkID(raw string) (uint, error) {
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		return 0, huma.Error400BadRequest("invalid network id")
	}

	return uint(id), nil
}
