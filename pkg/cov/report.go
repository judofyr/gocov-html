// Copyright (c) 2012 The Gocov Authors.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS
// IN THE SOFTWARE.

package cov

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/axw/gocov"
	"github.com/matm/gocov-html/pkg/themes"
	"github.com/matm/gocov-html/pkg/types"
	"github.com/rotisserie/eris"
)

func unmarshalJSON(data []byte) (packages []*gocov.Package, err error) {
	result := &struct{ Packages []*gocov.Package }{}
	err = json.Unmarshal(data, result)
	if err == nil {
		packages = result.Packages
	}
	return
}

type report struct {
	packages   []*gocov.Package
	stylesheet string // absolute path to CSS
}

type reverse struct {
	sort.Interface
}

func (r reverse) Less(i, j int) bool {
	return r.Interface.Less(j, i)
}

// NewReport creates a new report.
func newReport() (r *report) {
	r = &report{}
	return
}

// AddPackage adds a package's coverage information to the report.
func (r *report) addPackage(p *gocov.Package) {
	i := sort.Search(len(r.packages), func(i int) bool {
		return r.packages[i].Name >= p.Name
	})
	if i < len(r.packages) && r.packages[i].Name == p.Name {
		r.packages[i].Accumulate(p)
	} else {
		head := r.packages[:i]
		tail := append([]*gocov.Package{p}, r.packages[i:]...)
		r.packages = append(head, tail...)
	}
}

// Clear clears the coverage information from the report.
func (r *report) clear() {
	r.packages = nil
}

func buildReportPackage(pkg *gocov.Package) types.ReportPackage {
	rv := types.ReportPackage{
		Pkg:       pkg,
		Functions: make(types.ReportFunctionList, len(pkg.Functions)),
	}
	for i, fn := range pkg.Functions {
		reached := 0
		for _, stmt := range fn.Statements {
			if stmt.Reached > 0 {
				reached++
			}
		}
		rv.Functions[i] = types.ReportFunction{Function: fn, StatementsReached: reached}
		rv.TotalStatements += len(fn.Statements)
		rv.ReachedStatements += reached
	}
	sort.Sort(reverse{rv.Functions})
	return rv
}

// PrintReport prints a coverage report to the given writer.
func printReport(w io.Writer, r *report) error {
	theme := themes.Current()
	data := theme.Data()

	css := data.Style
	if len(r.stylesheet) > 0 {
		// Inline CSS.
		f, err := os.Open(r.stylesheet)
		if err != nil {
			return eris.Wrap(err, "print report")
		}
		style, err := ioutil.ReadAll(f)
		if err != nil {
			return eris.Wrap(err, "read style")
		}
		css = string(style)
	}
	reportPackages := make(types.ReportPackageList, len(r.packages))
	pkgNames := make([]string, len(r.packages))
	for i, pkg := range r.packages {
		reportPackages[i] = buildReportPackage(pkg)
		pkgNames[i] = pkg.Name
	}

	data.Style = css
	data.Packages = reportPackages
	data.Command = fmt.Sprintf("gocov test %s | gocov-html -t %s", strings.Join(pkgNames, " "), theme.Name())

	if len(reportPackages) > 1 {
		rv := types.ReportPackage{
			Pkg: &gocov.Package{Name: "Report Total"},
		}
		for _, rp := range reportPackages {
			rv.ReachedStatements += rp.ReachedStatements
			rv.TotalStatements += rp.TotalStatements
		}
		data.Overview = &rv
	}
	err := theme.Template().Execute(w, data)
	return eris.Wrap(err, "execute template")
}

func exists(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		return false, err
	}
	return true, nil
}

// HTMLReportCoverage outputs an HTML report on stdout by
// parsing JSON data generated by axw/gocov. The css parameter
// is an absolute path to a custom stylesheet. Use an empty
// string to use the default stylesheet available.
func HTMLReportCoverage(r io.Reader, css string) error {
	t0 := time.Now()
	report := newReport()

	// Custom stylesheet?
	stylesheet := ""
	if css != "" {
		if _, err := exists(css); err != nil {
			return eris.Wrap(err, "stylesheet")
		}
		stylesheet = css
	}
	report.stylesheet = stylesheet

	data, err := ioutil.ReadAll(r)
	if err != nil {
		return eris.Wrap(err, "read coverage data")
	}

	packages, err := unmarshalJSON(data)
	if err != nil {
		return eris.Wrap(err, "unmarshal coverage data")
	}

	for _, pkg := range packages {
		report.addPackage(pkg)
	}
	fmt.Println()
	err = printReport(os.Stdout, report)
	fmt.Fprintf(os.Stderr, "Took %v\n", time.Since(t0))
	return eris.Wrap(err, "HTML report")
}