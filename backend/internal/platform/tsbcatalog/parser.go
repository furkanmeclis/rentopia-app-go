package tsbcatalog

import (
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// ErrInvalidFile is returned when the workbook does not look like a TSB list.
var ErrInvalidFile = errors.New("tsbcatalog: invalid TSB kasko list")

// maxHeaderScan bounds how many leading rows are searched for the header row.
const maxHeaderScan = 10

// Row is one type line of the TSB list.
type Row struct {
	BrandCode string
	BrandName string
	TypeCode  string
	TypeName  string
	// Values maps model year to kasko value in TRY. Years whose value is 0
	// (the type was not sold that year) are omitted.
	Values map[int]int64
}

// Years returns the model years with a value, newest first.
func (r Row) Years() []int {
	out := make([]int, 0, len(r.Values))
	for y := range r.Values {
		out = append(out, y)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out
}

// Sheet is a parsed TSB workbook.
type Sheet struct {
	// PeriodLabel is the raw label from A1, e.g. "Ekim 2026".
	PeriodLabel string
	// Period is the normalized "YYYY-MM" period ("" when A1 is unreadable).
	Period string
	// Years lists the year columns found in the header, in sheet order.
	Years []int
	Rows  []Row
}

type columnMap struct {
	brandCode, typeCode, brandName, typeName int
	years                                    map[int]int // column index -> year
}

// Parse reads a TSB kasko list workbook. The period comes from cell A1; the
// header row is located by its column titles, so year columns may shift from
// one month to the next.
func Parse(r io.Reader) (*Sheet, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidFile, err)
	}
	defer func() { _ = f.Close() }()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("%w: workbook has no sheets", ErrInvalidFile)
	}
	sheetName := sheets[0]
	for _, s := range sheets {
		if strings.EqualFold(strings.TrimSpace(s), "Smarka") {
			sheetName = s
			break
		}
	}

	rows, err := f.Rows(sheetName)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidFile, err)
	}
	defer func() { _ = rows.Close() }()

	out := &Sheet{}
	var cols *columnMap
	seen := map[[2]string]struct{}{}
	brandNames := map[string]string{}
	line := 0
	for rows.Next() {
		line++
		cells, err := rows.Columns(excelize.Options{RawCellValue: true})
		if err != nil {
			return nil, fmt.Errorf("%w: row %d: %v", ErrInvalidFile, line, err)
		}
		if line == 1 && len(cells) > 0 {
			out.PeriodLabel = strings.TrimSpace(cells[0])
			if p, perr := ParsePeriod(out.PeriodLabel); perr == nil {
				out.Period = p
			}
		}
		if cols == nil {
			if m, ok := detectHeader(cells); ok {
				cols = m
				out.Years = sortedYears(m.years)
				continue
			}
			if line >= maxHeaderScan {
				return nil, fmt.Errorf("%w: header row (Marka Kodu / Tip Kodu / Marka Adı / Tip Adı) not found", ErrInvalidFile)
			}
			continue
		}
		row, ok, err := parseRow(cells, cols)
		if err != nil {
			return nil, fmt.Errorf("%w: row %d: %v", ErrInvalidFile, line, err)
		}
		if !ok {
			continue
		}
		key := [2]string{row.BrandCode, row.TypeCode}
		if _, dup := seen[key]; dup {
			return nil, fmt.Errorf("%w: row %d: duplicate brand/type code %s/%s", ErrInvalidFile, line, row.BrandCode, row.TypeCode)
		}
		seen[key] = struct{}{}
		if prev, ok := brandNames[row.BrandCode]; ok && prev != row.BrandName {
			return nil, fmt.Errorf("%w: row %d: brand code %s has two names (%q, %q)", ErrInvalidFile, line, row.BrandCode, prev, row.BrandName)
		}
		brandNames[row.BrandCode] = row.BrandName
		out.Rows = append(out.Rows, row)
	}
	if err := rows.Error(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidFile, err)
	}
	if cols == nil {
		return nil, fmt.Errorf("%w: header row not found", ErrInvalidFile)
	}
	if len(out.Rows) == 0 {
		return nil, fmt.Errorf("%w: no data rows", ErrInvalidFile)
	}
	return out, nil
}

func detectHeader(cells []string) (*columnMap, bool) {
	m := &columnMap{brandCode: -1, typeCode: -1, brandName: -1, typeName: -1, years: map[int]int{}}
	for i, c := range cells {
		h := foldTurkish(strings.Join(strings.Fields(c), " "))
		switch h {
		case "marka kodu":
			m.brandCode = i
		case "tip kodu":
			m.typeCode = i
		case "marka adi":
			m.brandName = i
		case "tip adi":
			m.typeName = i
		default:
			if y, ok := parseYearHeader(h); ok {
				m.years[i] = y
			}
		}
	}
	if m.brandCode < 0 || m.typeCode < 0 || m.brandName < 0 || m.typeName < 0 || len(m.years) == 0 {
		return nil, false
	}
	return m, true
}

func parseYearHeader(h string) (int, bool) {
	h = strings.TrimSuffix(h, ".0")
	if len(h) != 4 {
		return 0, false
	}
	y, err := strconv.Atoi(h)
	if err != nil || y < 1950 || y > 2100 {
		return 0, false
	}
	return y, true
}

func sortedYears(m map[int]int) []int {
	idx := make([]int, 0, len(m))
	for i := range m {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	out := make([]int, 0, len(idx))
	for _, i := range idx {
		out = append(out, m[i])
	}
	return out
}

func cell(cells []string, i int) string {
	if i < 0 || i >= len(cells) {
		return ""
	}
	return strings.TrimSpace(cells[i])
}

func parseRow(cells []string, cols *columnMap) (Row, bool, error) {
	row := Row{
		BrandCode: normalizeCode(cell(cells, cols.brandCode)),
		TypeCode:  normalizeCode(cell(cells, cols.typeCode)),
		BrandName: strings.Join(strings.Fields(cell(cells, cols.brandName)), " "),
		TypeName:  strings.Join(strings.Fields(cell(cells, cols.typeName)), " "),
		Values:    map[int]int64{},
	}
	if row.BrandCode == "" && row.TypeCode == "" && row.TypeName == "" {
		return Row{}, false, nil // blank line
	}
	if row.BrandCode == "" || row.TypeCode == "" || row.BrandName == "" || row.TypeName == "" {
		return Row{}, false, fmt.Errorf("missing brand/type code or name")
	}
	for i, year := range cols.years {
		raw := cell(cells, i)
		if raw == "" {
			continue
		}
		v, err := parseValue(raw)
		if err != nil {
			return Row{}, false, fmt.Errorf("year %d: %v", year, err)
		}
		if v > 0 {
			row.Values[year] = v
		}
	}
	return row, true, nil
}

// normalizeCode keeps codes as strings but drops a spurious ".0" that a
// numeric cell would carry.
func normalizeCode(s string) string {
	if strings.HasSuffix(s, ".0") {
		if _, err := strconv.Atoi(strings.TrimSuffix(s, ".0")); err == nil {
			return strings.TrimSuffix(s, ".0")
		}
	}
	return s
}

func parseValue(raw string) (int64, error) {
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if n < 0 {
			return 0, fmt.Errorf("negative value %q", raw)
		}
		return n, nil
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || f < 0 || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("invalid value %q", raw)
	}
	return int64(math.Round(f)), nil
}
