package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/krasin/cobra"
	"github.com/krasin/stl"
)

var scaleX float64
var outPath string

func fail(args ...interface{}) {
	fmt.Fprintln(os.Stderr, args...)
	os.Exit(1)
}

func openIn(files []string) (io.ReadCloser, error) {
	if len(files) == 0 {
		return os.Stdin, nil
	}
	if len(files) > 1 {
		return nil, errors.New("multiple input files are not supported yet")
	}
	return os.Open(files[0])
}

func openOut(path string) (io.WriteCloser, error) {
	if path == "" {
		return os.Stdout, nil
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
}

func info(cmd *cobra.Command, args []string) {
	r, err := openIn(args)
	if err != nil {
		fail(err)
	}
	defer r.Close()
	t, err := stl.Read(r)
	if err != nil {
		fail("Failed to read STL file:", err)
	}
	fmt.Printf("Triangles: %d\n", len(t))
	min, max := stl.BoundingBox(t)
	fmt.Printf("Bounding box: %v - %v\n", min, max)
}

func scale(cmd *cobra.Command, args []string) {
	r, err := openIn(args)
	if err != nil {
		fail(err)
	}
	defer r.Close()
	w, err := openOut(outPath)
	if err != nil {
		fail(err)
	}
	defer w.Close()

	t, err := stl.Read(r)
	if err != nil {
		fail("Failed to read STL file:", err)
	}
	for i := range t {
		tr := &t[i]
		for j := 0; j < 3; j++ {
			for k := 0; k < 3; k++ {
				tr.V[j][k] = float32(scaleX * float64(tr.V[j][k]))
			}
		}
	}
	if err := stl.Write(w, t); err != nil {
		fail("Failed to write STL file:", err)
	}
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
		Long: `info displays STL metrics, such as the number of triangles, bounding box, etc.
If no STL file is specified, it will read from stdin`,
		Run: info,
	}
	scaleCmd := &cobra.Command{
		Use:   "scale [STL file]",
		Short: "Scale mesh",
		Long: `scale multiplies all mesh vertices coordinates by the specified amount.
If no STL file is specified, it will read from stdin`,
		Run: scale,
	}

	scaleCmd.Flags().Float64VarP(&scaleX, "x", "x", 1, "Scale factor")
	scaleCmd.Flags().StringVarP(&outPath, "output", "o", "", "Output STL file. By default, it's stdout")
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(scaleCmd)
	rootCmd.Execute()
}
