package main

import (
	"fmt"
	"os"

	"github.com/krasin/cobra"
	"github.com/krasin/stl"
)

func fail(args ...interface{}) {
	fmt.Fprintln(os.Stderr, args...)
	os.Exit(1)
}

func info(cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		fail("STL file not specified")
	}
	f, err := os.Open(args[0])
	if err != nil {
		fail(err)
	}
	defer f.Close()
	t, err := stl.Read(f)
	if err != nil {
		fail("Failed to read STL file:", err)
	}
	fmt.Printf("Triangles: %d\n", len(t))
}

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
	infoCmd := &cobra.Command{
		Use:   "info [STL file]",
		Short: "STL file info",
		Long:  "info displays STL metrics, such as the number of triangles, bounding box, etc",
		Run:   info,
	}
	rootCmd.AddCommand(infoCmd)
	rootCmd.Execute()
}
