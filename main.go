package main

import (
	"fmt"

	"github.com/krasin/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "steel",
		Short: "A tool to tinker with STL files",
		Long:  "Command-line processor for STL files",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Steel -- a tool to tinker with STL files.")
			cmd.Usage()
		},
	}
	rootCmd.Execute()
}
