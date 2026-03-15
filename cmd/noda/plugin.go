package main

import (
	"fmt"
	"sort"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/spf13/cobra"
)

func newPluginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage plugins",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all registered plugins with prefixes and node counts",
		RunE: func(_ *cobra.Command, _ []string) error {
			// Build a real plugin registry from all core and service-only plugins
			pluginReg := registry.NewPluginRegistry()
			if err := registerCorePlugins(pluginReg); err != nil {
				return err
			}

			nodeReg := registry.NewNodeRegistry()
			for _, p := range pluginReg.All() {
				if err := nodeReg.RegisterFromPlugin(p); err != nil {
					return fmt.Errorf("register nodes from %q: %w", p.Name(), err)
				}
			}

			type pluginInfo struct {
				name   string
				prefix string
				nodes  int
			}

			var infos []pluginInfo
			for _, p := range pluginReg.All() {
				count := nodeReg.CountByPrefix(p.Prefix())
				infos = append(infos, pluginInfo{
					name:   p.Name(),
					prefix: p.Prefix(),
					nodes:  count,
				})
			}

			sort.Slice(infos, func(i, j int) bool {
				return infos[i].name < infos[j].name
			})

			fmt.Printf("%-15s  %-10s  %s\n", "PLUGIN", "PREFIX", "NODES")
			fmt.Println("-------------------------------------------")
			total := 0
			for _, p := range infos {
				fmt.Printf("%-15s  %-10s  %d\n", p.name, p.prefix, p.nodes)
				total += p.nodes
			}
			fmt.Printf("\n%d plugins, %d node types\n", len(infos), total)
			return nil
		},
	}

	cmd.AddCommand(listCmd)
	return cmd
}
