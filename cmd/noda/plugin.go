package main

import (
	"fmt"

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
		Run: func(_ *cobra.Command, _ []string) {
			type pluginInfo struct {
				name   string
				prefix string
				nodes  int
			}

			infos := []pluginInfo{
				{"control", "control", 3},
				{"transform", "transform", 4},
				{"util", "util", 4},
				{"workflow", "workflow", 2},
				{"response", "response", 3},
				{"db", "db", 4},
				{"cache", "cache", 4},
				{"event", "event", 1},
				{"storage", "storage", 3},
				{"upload", "upload", 1},
				{"image", "image", 2},
				{"http", "http", 1},
				{"email", "email", 1},
				{"ws", "ws", 1},
				{"sse", "sse", 1},
				{"wasm", "wasm", 2},
				{"stream", "stream", 0},
				{"pubsub", "pubsub", 0},
			}

			fmt.Printf("%-15s  %-10s  %s\n", "PLUGIN", "PREFIX", "NODES")
			fmt.Println("-------------------------------------------")
			total := 0
			for _, p := range infos {
				fmt.Printf("%-15s  %-10s  %d\n", p.name, p.prefix, p.nodes)
				total += p.nodes
			}
			fmt.Printf("\n%d plugins, %d node types\n", len(infos), total)
		},
	}

	cmd.AddCommand(listCmd)
	return cmd
}
