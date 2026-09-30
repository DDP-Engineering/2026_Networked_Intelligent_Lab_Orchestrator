package modbus

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

const MaxMapDocumentBytes = 1 << 20

var mapCSVHeader = []string{"schema_version", "map_id", "map_revision", "map_owner_id", "map_name", "point_id", "point_name", "description", "area", "address_base", "address", "type", "width", "order", "unit", "scale", "offset", "precision", "access", "write_min", "write_max", "labels", "padding"}

// RowError identifies one import issue without disclosing a partial publication.
type RowError struct {
	Row   int    `json:"row"`
	Field string `json:"field"`
	Code  string `json:"code"`
}

func csvSafe(s string) string {
	if strings.HasPrefix(s, "~") {
		return "~" + s
	}
	trimmed := strings.TrimLeft(s, " \t")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "~" + s
	}
	return s
}
func csvUnsafe(s string) string { return strings.TrimPrefix(s, "~") }

// ExportMap emits a complete, bounded, schema-versioned map document.
func ExportMap(item Map, format string) ([]byte, error) {
	m, err := item.Canonical()
	if err != nil {
		return nil, err
	}
	if format == "json" {
		return json.Marshal(m)
	}
	if format != "csv" {
		return nil, ErrInvalid
	}
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if writer.Write(mapCSVHeader) != nil {
		return nil, ErrInvalid
	}
	points := m.Points
	if len(points) == 0 {
		points = []Point{{}}
	}
	for _, p := range points {
		labels := map[string]string{}
		for k, v := range p.Labels {
			labels[k] = csvSafe(v)
		}
		labelsRaw, _ := json.Marshal(labels)
		row := []string{"1", m.ID, strconv.FormatInt(m.Revision, 10), m.OwnerID, csvSafe(m.Name), p.ID, csvSafe(p.Name), csvSafe(p.Description), p.Area, "zero", strconv.Itoa(p.Address), p.Type, strconv.Itoa(p.Width), p.Order, csvSafe(p.Unit), string(p.Scale), string(p.Offset), "", p.Access, "", "", string(labelsRaw), p.Padding}
		if p.Precision != nil {
			row[17] = strconv.Itoa(*p.Precision)
		}
		if p.WriteMin != nil {
			row[19] = string(*p.WriteMin)
		}
		if p.WriteMax != nil {
			row[20] = string(*p.WriteMax)
		}
		if p.ID == "" {
			for i := 5; i < len(row); i++ {
				row[i] = ""
			}
		}
		if writer.Write(row) != nil {
			return nil, ErrInvalid
		}
	}
	writer.Flush()
	if writer.Error() != nil || buf.Len() > MaxMapDocumentBytes {
		return nil, ErrInvalid
	}
	return buf.Bytes(), nil
}

func strictMapJSON(data []byte) (Map, error) {
	var item Map
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || fields == nil {
		return item, ErrInvalid
	}
	for _, name := range []string{"schema_version", "id", "revision", "owner_id", "name", "points"} {
		if _, ok := fields[name]; !ok {
			return item, ErrInvalid
		}
	}
	if len(bytes.TrimSpace(fields["points"])) == 0 || bytes.TrimSpace(fields["points"])[0] != '[' {
		return item, ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&item); err != nil {
		return item, ErrInvalid
	}
	if decoder.Decode(new(any)) != io.EOF {
		return item, ErrInvalid
	}
	return item.Canonical()
}

func csvPoint(row []string) (Point, string, error) {
	point := Point{ID: row[5], Name: csvUnsafe(row[6]), Description: csvUnsafe(row[7]), Area: row[8], Type: row[11], Order: row[13], Unit: csvUnsafe(row[14]), Scale: json.Number(row[15]), Offset: json.Number(row[16]), Access: row[18], Padding: row[22]}
	var err error
	address, e := strconv.Atoi(row[10])
	if e != nil {
		return Point{}, "address", ErrInvalid
	}
	point.Address, err = WireAddress(point.Area, row[9], address)
	if err != nil {
		return Point{}, "address", err
	}
	point.Width, err = strconv.Atoi(row[12])
	if err != nil {
		return Point{}, "width", ErrInvalid
	}
	if row[17] != "" {
		n, e := strconv.Atoi(row[17])
		if e != nil {
			return Point{}, "precision", ErrInvalid
		}
		point.Precision = &n
	}
	if row[19] != "" {
		n := json.Number(row[19])
		point.WriteMin = &n
	}
	if row[20] != "" {
		n := json.Number(row[20])
		point.WriteMax = &n
	}
	if row[21] == "" {
		point.Labels = map[string]string{}
	} else {
		decoder := json.NewDecoder(strings.NewReader(row[21]))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&point.Labels) != nil || decoder.Decode(new(any)) != io.EOF {
			return Point{}, "labels", ErrInvalid
		}
		for key, label := range point.Labels {
			point.Labels[key] = csvUnsafe(label)
		}
	}
	canonical, err := point.Canonical()
	if err != nil {
		return Point{}, "point", err
	}
	return canonical, "", nil
}

// ParseMapDocument reports all bounded row errors and never changes storage.
func ParseMapDocument(format string, data []byte) (Map, []RowError) {
	fail := func(row int, field, code string) (Map, []RowError) {
		return Map{}, []RowError{{Row: row, Field: field, Code: code}}
	}
	if len(data) > MaxMapDocumentBytes || !utf8.Valid(data) {
		return fail(0, "document", "limit_or_utf8")
	}
	if format == "json" {
		item, err := strictMapJSON(data)
		if err != nil {
			return fail(0, "document", "invalid")
		}
		return item, nil
	}
	if format != "csv" {
		return fail(0, "format", "unsupported")
	}
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = len(mapCSVHeader)
	header, err := reader.Read()
	if err != nil {
		return fail(1, "header", "invalid")
	}
	for i, v := range mapCSVHeader {
		if header[i] != v {
			return fail(1, "header", "invalid")
		}
	}
	var item Map
	item.Points = []Point{}
	issues := []RowError{}
	rowNumber := 1
	metadata := []string(nil)
	seenPoints := map[string]bool{}
	for {
		row, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		rowNumber++
		if readErr != nil {
			issues = append(issues, RowError{rowNumber, "row", "invalid_csv"})
			break
		}
		size := 0
		for _, cell := range row {
			size += len(cell)
		}
		if size > 32<<10 {
			issues = append(issues, RowError{rowNumber, "row", "record_limit"})
			continue
		}
		current := []string{row[0], row[1], row[2], row[3], row[4]}
		if metadata == nil {
			metadata = current
			item.SchemaVersion, err = strconv.Atoi(row[0])
			if err != nil || item.SchemaVersion != 1 {
				issues = append(issues, RowError{rowNumber, "schema_version", "invalid"})
			}
			item.ID = row[1]
			item.Revision, err = strconv.ParseInt(row[2], 10, 64)
			if err != nil || item.Revision < 1 || item.Revision > 2147483647 {
				issues = append(issues, RowError{rowNumber, "map_revision", "invalid"})
			}
			item.OwnerID = row[3]
			item.Name = csvUnsafe(row[4])
			if !identifier.MatchString(item.ID) {
				issues = append(issues, RowError{rowNumber, "map_id", "invalid"})
			}
			if !identifier.MatchString(item.OwnerID) {
				issues = append(issues, RowError{rowNumber, "map_owner_id", "invalid"})
			}
			if !validName(item.Name) {
				issues = append(issues, RowError{rowNumber, "map_name", "invalid"})
			}
		} else {
			for i := range metadata {
				if metadata[i] != current[i] {
					issues = append(issues, RowError{rowNumber, mapCSVHeader[i], "metadata_mismatch"})
				}
			}
		}
		if row[5] == "" {
			if rowNumber != 2 {
				issues = append(issues, RowError{rowNumber, "point_id", "empty_row"})
			}
			continue
		}
		if len(item.Points) >= 4096 {
			issues = append(issues, RowError{rowNumber, "points", "point_limit"})
			continue
		}
		if seenPoints[row[5]] {
			issues = append(issues, RowError{rowNumber, "point_id", "duplicate"})
			continue
		}
		point, field, e := csvPoint(row)
		if e != nil {
			issues = append(issues, RowError{rowNumber, field, "invalid"})
			continue
		}
		seenPoints[point.ID] = true
		item.Points = append(item.Points, point)
	}
	if rowNumber == 1 {
		return fail(2, "row", "missing")
	}
	if len(issues) > 0 {
		return Map{}, issues
	}
	canonical, err := item.Canonical()
	if err != nil {
		return fail(0, "map", "invalid")
	}
	return canonical, nil
}
