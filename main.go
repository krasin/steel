package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/krasin/cobra"
	"github.com/krasin/stl"
)

const sliceThreshold = 0.001
const cutThreshold = 0.0001

var (
	scaleX  float64
	coordX  float64
	coordY  float64
	coordZ  float64
	outPath string
	verbose bool
)

func fail(args ...interface{}) {
	fmt.Fprintln(os.Stderr, args...)
	os.Exit(1)
}

func openIn(files []string) (string, io.ReadCloser, error) {
	if len(files) == 0 {
		return "", os.Stdin, nil
	}
	if len(files) > 1 {
		return "", nil, errors.New("multiple input files are not supported yet")
	}
	f, err := os.Open(files[0])
	return files[0], f, err
}

func openOut(path string) (io.WriteCloser, error) {
	if path == "" {
		return os.Stdout, nil
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
}

func info(cmd *cobra.Command, args []string) {
	name, r, err := openIn(args)
	if err != nil {
		fail(err)
	}
	defer r.Close()
	t, err := stl.Read(r)
	if err != nil {
		fail(fmt.Sprintf("Failed to read STL file %q: %v", name, err))
	}
	fmt.Printf("File: %s\n", name)
	fmt.Printf("Triangles: %d\n", len(t))
	min, max := stl.BoundingBox(t)
	fmt.Printf("Bounding box: %v - %v\n", min, max)
}

func scale(cmd *cobra.Command, args []string) {
	_, r, err := openIn(args)
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
	if err := stl.WriteBinary(w, t); err != nil {
		fail("Failed to write STL file:", err)
	}
}

func slice(cmd *cobra.Command, args []string) {
	_, r, err := openIn(args)
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
	intersect := func(p0, p1 stl.Point) (res stl.Point) {
		alpha := (sv - p0[si]) / (p1[si] - p0[si])
		for i := 0; i < 3; i++ {
			res[i] = p0[i] + alpha*(p1[i]-p0[i])
		}
		return
	}

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
			if verbose {
				fmt.Fprintf(w, "<!-- skip a triangle; it's below: %v -->\n", tr.V)
			}
			continue
		}
		if more(tr.V[0]) && more(tr.V[1]) && more(tr.V[2]) {
			if verbose {
				fmt.Fprintf(w, "<!-- skip a triangle; it's above: %v -->\n", tr.V)
			}
			continue
		}

		// Now, check if we need to draw the whole triangle
		if eq(tr.V[0]) && eq(tr.V[1]) && eq(tr.V[2]) {
			fmt.Fprintf(w, "<path d=\"M%s L%s L%s L%s\"/>\n", pxy(tr.V[0]), pxy(tr.V[1]), pxy(tr.V[2]), pxy(tr.V[0]))
			continue
		}

		// OK, it's the line. Two cases: line is a triangle side, or it's not.
		was := false
		for i := 0; i < 3; i++ {
			j := (i + 1) % 3
			k := (i + 2) % 3
			// First, let's check if it's a triangle side.
			if eq(tr.V[i]) && eq(tr.V[j]) {
				fmt.Fprintf(w, "<path d='M%s L%s' />\n", pxy(tr.V[i]), pxy(tr.V[j]))
				was = true
				break
			}
			if less(tr.V[i]) && less(tr.V[j]) || more(tr.V[i]) && more(tr.V[j]) {
				// Since this triangle is known to intersect the slice plane, the k'th vertex is on the other side.
				// and we need to find intersection points on i-k and j-k triangle sides.
				p0 := intersect(tr.V[i], tr.V[k])
				p1 := intersect(tr.V[j], tr.V[k])
				fmt.Fprintf(w, "<path d='M%s L%s' />\n", pxy(p0), pxy(p1))
				was = true
			}
		}
		if was {
			continue
		}

		// Just a dot
		if verbose {
			fmt.Fprintf(w, "<!-- it's just a dot: %v -->\n", tr)
		}
	}

	// Write SVG footer
	fmt.Fprintln(w, "</g>")
	fmt.Fprintln(w, "</svg>")
}

// uniqVertices removes equal points which are located in the adjacent positions.
func uniqVertices(v []stl.Point) []stl.Point {
	if len(v) == 0 {
		return nil
	}
	res := []stl.Point{v[0]}
	for i := 1; i < len(v); i++ {
		if v[i] == res[len(res)-1] {
			continue
		}
		res = append(res, v[i])
	}
	return res
}

// trimTriangleBelow trims the triangle with the provided surface and returns the list of resulting triangles.
func trimTriangleBelow(tr stl.Triangle, below, above func(p stl.Point) bool, intersect func(p1, p2 stl.Point) stl.Point) []stl.Triangle {
	eq := func(p stl.Point) bool { return !above(p) && !below(p) }

	var v []stl.Point
	for i := 0; i < 3; i++ {
		cur := tr.V[i]
		next := tr.V[(i+1)%3]
		// full edge will be in the result
		if !above(cur) && !above(next) {
			v = append(v, cur, next)
			continue
		}
		// edge is fully above, ignore both vertices
		if above(cur) && above(next) {
			continue
		}

		// one vertice is above, another is on the border
		if eq(cur) && above(next) {
			v = append(v, cur)
			continue
		}
		if above(cur) && eq(next) {
			v = append(v, next)
			continue
		}
		// the remaining case: one is below, another is above
		if below(cur) && above(next) {
			v = append(v, cur, intersect(cur, next))
			continue
		}
		if above(cur) && below(next) {
			v = append(v, intersect(cur, next), next)
			continue
		}
		panic("unreachable. If this code is executed, there's a bug in the code related how the split surface is defined.")
	}
	// remove duplicates
	v = uniqVertices(v)
	// fix up: it could be that the first and the last vertices are the same; uniqVertices will not detect them
	if len(v) > 1 && v[0] == v[len(v)-1] {
		v = v[:len(v)-1]
	}

	// now, we need to calculate the number of vertices.

	// 0 triangles
	if len(v) < 3 {
		return nil
	}
	// 1 triangle
	if len(v) == 3 {
		return []stl.Triangle{
			{
				N: tr.N,
				V: [3]stl.Point{v[0], v[1], v[2]},
			},
		}
	}
	// 2 triangles
	if len(v) == 4 {
		return []stl.Triangle{
			{
				N: tr.N,
				V: [3]stl.Point{v[0], v[1], v[2]},
			},
			{
				N: tr.N,
				V: [3]stl.Point{v[2], v[3], v[0]},
			},
		}
	}
	panic(fmt.Errorf("unreachable. len(v) = %d. If this code is executed, there's a bug in the code that splits a triangle with the surfaces provided", len(v)))
}

func cut(cmd *cobra.Command, args []string) {
	_, r, err := openIn(args)
	if err != nil {
		fail(err)
	}
	defer r.Close()

	if outPath == "" {
		fail(errors.New("--output is not specified"))
	}

	// Find output file base. For example: /home/user/lala.stl -> /home/user/lala, and
	// then it will become /home/user/{lala001.stl,lala002.stl}.
	outExt := filepath.Ext(outPath)
	outBase := outPath[:len(outPath)-len(outExt)]

	// Read input STL
	t, err := stl.Read(r)
	if err != nil {
		fail("Failed to read STL file:", err)
	}

	// by default, cut with XY plane at z = 0
	si := 2
	var sv float32
	cnt := 0

	for i, v := range []float64{coordX, coordY, coordZ} {
		if v != 0 {
			si = i
			sv = float32(v)
			cnt++
		}
	}
	if cnt > 1 {
		fail("More than one coord is specified: x: %f, y: %f, z: %f", coordX, coordY, coordZ)
	}

	min, max := stl.BoundingBox(t)
	eps := (max[si] - min[si]) * cutThreshold

	below := func(p stl.Point) bool { return p[si] < sv-eps }
	above := func(p stl.Point) bool { return p[si] > sv+eps }

	intersect := func(p0, p1 stl.Point) (res stl.Point) {
		alpha := (sv - p0[si]) / (p1[si] - p0[si])
		for i := 0; i < 3; i++ {
			res[i] = p0[i] + alpha*(p1[i]-p0[i])
		}
		return
	}

	// We'll have two output parts: below and above.
	parts := make([][]stl.Triangle, 2)

	for _, tr := range t {
		// For each triangle, we have the options:
		// 1. put into bottom part (all vertices are not above)
		// 2. put into upper part (all vertices are not below)
		// 3. put into both parts (all vertices are equal) -- special case for the two rules above
		// 4. split triangle into two parts, if some vertices are above, and some are below

		simple := false

		if !above(tr.V[0]) && !above(tr.V[1]) && !above(tr.V[2]) {
			parts[0] = append(parts[0], tr)
			simple = true
		}
		if !below(tr.V[0]) && !below(tr.V[1]) && !below(tr.V[2]) {
			parts[1] = append(parts[1], tr)
			simple = true
		}
		if simple {
			continue
		}

		// We'll need to split the triangle into bottom and upper parts.
		tmp := trimTriangleBelow(tr, below, above, intersect)
		parts[0] = append(parts[0], tmp...)

		tmp = trimTriangleBelow(tr, above, below, intersect)
		parts[1] = append(parts[1], tmp...)
	}

	// Now, we need to save both parts.
	for i := 0; i < 2; i++ {
		w, err := openOut(fmt.Sprintf("%s%03d%s", outBase, i, outExt))
		if err != nil {
			fail("Failed to open output file: ", err)
		}

		if err := stl.WriteASCII(w, parts[i]); err != nil {
			w.Close()
			fail("Failed to save an output STL: ", err)
		}
		if err := w.Close(); err != nil {
			fail("Failed to close the output file: ", err)
		}
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
	sliceCmd.Flags().BoolVarP(&verbose, "verbose", "v", false,
		"Verbosity. If verbose, skipped triangles will leave comments in the output SVG file.")
	sliceCmd.Flags().Float64VarP(&coordX, "x", "x", 0, "If specified, it will slice with YZ plane at specified x.")
	sliceCmd.Flags().Float64VarP(&coordY, "y", "y", 0, "If specified, it will slice with XZ plane at specified y.")
	sliceCmd.Flags().Float64VarP(&coordZ, "z", "z", 0, "If specified, it will slice with XY plane at specified z.")
	rootCmd.AddCommand(sliceCmd)

	cutCmd := &cobra.Command{
		Use:   "cut [STL file]",
		Short: "Cut mesh by a plane into two parts",
		Long: `cut mesh by a specified plan into two parts and save them as STL.
If no STL file is specified, it will read from stdin.`,
		Run: cut,
	}
	cutCmd.Flags().StringVarP(&outPath, "output", "o", "", "The base for output STL files. For example, /home/user/lala.stl will result in /home/user/lala001.stl and /home/user/lala002.stl.")
	cutCmd.Flags().Float64VarP(&coordX, "x", "x", 0, "If specified, it will сut with YZ plane at specified x.")
	cutCmd.Flags().Float64VarP(&coordY, "y", "y", 0, "If specified, it will cut with XZ plane at specified y.")
	cutCmd.Flags().Float64VarP(&coordZ, "z", "z", 0, "If specified, it will cut with XY plane at specified z.")
	rootCmd.AddCommand(cutCmd)

	rootCmd.Execute()
}
