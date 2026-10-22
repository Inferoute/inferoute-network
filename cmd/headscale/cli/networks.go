package cli

import (
	"context"
	"fmt"
	"net/http"

	clientv1 "github.com/juanfont/headscale/gen/client/v1"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(networkCmd)
	networkCmd.AddCommand(createNetworkCmd)
	networkCmd.AddCommand(listNetworksCmd)
	networkCmd.AddCommand(destroyNetworkCmd)

	createNetworkCmd.Flags().String("name", "", "Network name")
	mustMarkRequired(createNetworkCmd, "name")
	destroyNetworkCmd.Flags().String("name", "", "Network name")
	mustMarkRequired(destroyNetworkCmd, "name")
}

var networkCmd = &cobra.Command{
	Use:     "networks",
	Short:   "Manage tailnets",
	Aliases: []string{"network"},
}

var createNetworkCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a network and its default user",
	RunE: clientRunE(func(ctx context.Context, client *clientv1.ClientWithResponses, cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")

		resp, err := client.CreateNetworkWithResponse(ctx, clientv1.CreateNetworkJSONRequestBody{Name: name})
		if err != nil {
			return fmt.Errorf("creating network: %w", err)
		}

		if resp.StatusCode() != http.StatusOK {
			return apiError(resp.StatusCode(), resp.ApplicationproblemJSONDefault)
		}

		network := resp.JSON200.Network

		return printOutput(cmd, network, fmt.Sprintf("Network created: %s (%s)", network.Name, network.Id))
	}),
}

var listNetworksCmd = &cobra.Command{
	Use:     cmdList,
	Short:   "List networks",
	Aliases: []string{"ls", cmdShow},
	RunE: clientRunE(func(ctx context.Context, client *clientv1.ClientWithResponses, cmd *cobra.Command, args []string) error {
		resp, err := client.ListNetworksWithResponse(ctx)
		if err != nil {
			return fmt.Errorf("listing networks: %w", err)
		}

		if resp.StatusCode() != http.StatusOK {
			return apiError(resp.StatusCode(), resp.ApplicationproblemJSONDefault)
		}

		networks := resp.JSON200.Networks

		return printListOutput(cmd, networks, func() error {
			rows := make([][]string, 0, len(networks))
			for _, network := range networks {
				rows = append(rows, []string{network.Id, network.Name})
			}

			return renderTable([]string{"ID", "Name"}, rows)
		})
	}),
}

var destroyNetworkCmd = &cobra.Command{
	Use:     "destroy",
	Short:   "Delete a network and its nodes",
	Aliases: []string{aliasDel, "rm"},
	RunE: clientRunE(func(ctx context.Context, client *clientv1.ClientWithResponses, cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")

		id, err := networkIDByName(ctx, client, name)
		if err != nil {
			return err
		}

		resp, err := client.DeleteNetworkWithResponse(ctx, id)
		if err != nil {
			return fmt.Errorf("deleting network: %w", err)
		}

		if resp.StatusCode() != http.StatusOK {
			return apiError(resp.StatusCode(), resp.ApplicationproblemJSONDefault)
		}

		return printOutput(cmd, resp.JSON200, "Network deleted")
	}),
}

func networkIDByName(ctx context.Context, client *clientv1.ClientWithResponses, name string) (string, error) {
	resp, err := client.ListNetworksWithResponse(ctx)
	if err != nil {
		return "", fmt.Errorf("listing networks: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return "", apiError(resp.StatusCode(), resp.ApplicationproblemJSONDefault)
	}

	for _, network := range resp.JSON200.Networks {
		if network.Name == name {
			return network.Id, nil
		}
	}

	return "", fmt.Errorf("network %q not found", name)
}
