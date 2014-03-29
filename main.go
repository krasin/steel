package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/krasin/cobra"
	"github.com/krasin/stl"
)

const sliceThreshold = 0.001

var (
	scaleX  float64
	coordX  float64
	coordY  float64
	coordZ  float64
	outPath string
)

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

func slice(cmd *cobra.Command, args []string) {
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

	// by default, slice with XY plane at z = 0
	si := 2
	sx := 0
	sy := 0
	var sv float32
	cnt := 0

	for i, v := range []float64{coordX, coordY, coordZ} {
		if v != 0 {
			si, sx, sy = i, (i+1)%3, (i+2)%3
			sv = float32(v)
			cnt++
		}
	}
	if cnt > 1 {
		fail("More than one coord is specified: x: %f, y: %f, z: %f", coordX, coordY, coordZ)
	}

	min, max := stl.BoundingBox(t)
	eps := (max[si] - min[si]) * sliceThreshold

	less := func(p stl.Point) bool { return p[si] < sv-eps }
	more := func(p stl.Point) bool { return p[si] > sv+eps }
	eq := func(p stl.Point) bool { return !less(p) && !more(p) }

	// SVG file will have units of 0.01 mm, and the input STL file is treated as mm.
	pmm := func(v float32) int { return int(v * 100) }
	width := pmm(max[sx] - min[sx])
	height := pmm(max[sy] - min[sy])

	// Write SVG header
	fmt.Fprintln(w, `<?xml version="1.0" encoding="UTF-8" standalone="no"?>`)
	fmt.Fprintln(w, `<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd">`)
	fmt.Fprintf(w, `<svg width="%fmm" height="%fmm" version="1.1" viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg">`,
		float64(width)/100, float64(height)/100, width, height)
	fmt.Fprintln(w)
	fmt.Fprintln(w, `<g fill="gray" stroke="black" stroke-width="10">`)

	xx := func(v float32) int { return pmm(v - min[sx]) }
	yy := func(v float32) int { return pmm(v - min[sy]) }
	pxy := func(p stl.Point) string { return fmt.Sprintf("%d,%d", xx(p[sx]), yy(p[sy])) }

	for _, tr := range t {
		// For each triangle, we have the options: skip, draw a line, draw the triangle.

		// First, let's detect the triangles to skip.
		if less(tr.V[0]) && less(tr.V[1]) && less(tr.V[2]) {
			fmt.Fprintf(w, "<!-- skip a triangle; it's below: %v -->\n", tr.V)
			continue
		}
		if more(tr.V[0]) && more(tr.V[1]) && more(tr.V[2]) {
			fmt.Fprintf(w, "<!-- skip a triangle; it's above: %v -->\n", tr.V)
			continue
		}

		// Now, check if we need to draw the whole triangle
		if eq(tr.V[0]) && eq(tr.V[1]) && eq(tr.V[2]) {
			fmt.Fprintf(w, "<path d=\"M%s L%s L%s L%s\"/>\n", pxy(tr.V[0]), pxy(tr.V[1]), pxy(tr.V[2]), pxy(tr.V[0]))
			continue
		}
		// We need to draw a line
		fmt.Fprintf(w, "<!-- here be a line -->\n")
	}

	// Write SVG footer
	fmt.Fprintln(w, "</g>")
	fmt.Fprintln(w, "</svg>")
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
	rootCmd.AddCommand(infoCmd)

	scaleCmd := &cobra.Command{
		Use:   "scale [STL file]",
		Short: "Scale mesh",
		Long: `scale multiplies all mesh vertices coordinates by the specified amount.
If no STL file is specified, it will read from stdin.`,
		Run: scale,
	}
	scaleCmd.Flags().Float64VarP(&scaleX, "x", "x", 1, "Scale factor")
	scaleCmd.Flags().StringVarP(&outPath, "output", "o", "", "Output STL file. By default, it's stdout.")
	rootCmd.AddCommand(scaleCmd)

	sliceCmd := &cobra.Command{
		Use:   "slice [STL file]",
		Short: "Slice mesh by a plane to SVG",
		Long: `slice mesh by a specified plane and render to SVG graphics.
If no STL file is specified, it will read from stdin.`,
		Run: slice,
	}
	sliceCmd.Flags().StringVarP(&outPath, "output", "o", "", "Output SVG file. By default, it's stdout.")
	sliceCmd.Flags().Float64VarP(&coordX, "x", "x", 0, "If specified, it will slice with YZ plane at specified x.")
	sliceCmd.Flags().Float64VarP(&coordY, "y", "y", 0, "If specified, it will slice with XZ plane at specified y.")
	sliceCmd.Flags().Float64VarP(&coordZ, "z", "z", 0, "If specified, it will slice with XY plane at specified z.")
	rootCmd.AddCommand(sliceCmd)

	rootCmd.Execute()
}
