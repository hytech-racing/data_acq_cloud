package matfile

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"gonum.org/v1/hdf5"
)

func TestWriterWritesMatlabFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "signals.mat")

	writer, err := NewWriter(path)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	chunkOne := map[string]map[string]interface{}{
		"Topic": {
			"sig":    newFloat32Column([]float32{1.5, 2.5, 3.5}, []float64{0, 0.1, 0.2}),
			"nested": map[string]interface{}{"count": newInt32Column([]int32{1, 2, 3}, []float64{0, 0.1, 0.2})},
			"flag":   newBoolColumn([]bool{true, false, true}, []float64{0, 0.1, 0.2}),
		},
	}
	chunkTwo := map[string]map[string]interface{}{
		"Topic": {
			"sig":    newFloat32Column([]float32{4.5, 5.5}, []float64{0.3, 0.4}),
			"nested": map[string]interface{}{"count": newInt32Column([]int32{4, 5}, []float64{0.3, 0.4})},
			"flag":   newBoolColumn([]bool{false, true}, []float64{0.3, 0.4}),
		},
	}

	if err := writer.WriteChunk(chunkOne); err != nil {
		t.Fatalf("WriteChunk one: %v", err)
	}
	if err := writer.WriteChunk(chunkTwo); err != nil {
		t.Fatalf("WriteChunk two: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read MAT file: %v", err)
	}
	if len(raw) < 516 {
		t.Fatalf("MAT file is too short: %d bytes", len(raw))
	}
	if !bytes.HasPrefix(raw[:512], []byte("MATLAB 7.3 MAT-file, Platform: GLNXA64, Created on: ")) {
		t.Fatalf("unexpected MAT header: %q", raw[:116])
	}
	if !bytes.Equal(raw[124:126], []byte{0x00, 0x02}) {
		t.Fatalf("MAT version = % x, want 00 02", raw[124:126])
	}
	if !bytes.Equal(raw[126:128], []byte{'I', 'M'}) {
		t.Fatalf("MAT endian marker = % x, want 49 4d", raw[126:128])
	}
	if !bytes.Equal(raw[512:516], []byte{0x89, 'H', 'D', 'F'}) {
		t.Fatalf("missing HDF5 signature at offset 512: % x", raw[512:516])
	}

	file, err := hdf5.OpenFile(path, hdf5.F_ACC_RDONLY)
	if err != nil {
		t.Fatalf("open MAT file: %v", err)
	}
	t.Cleanup(func() { file.Close() })

	data := openTestGroup(t, file, "data", "/data")
	assertMatlabClass(t, data, "struct", "/data")

	refs := openTestGroup(t, file, "#refs#", "/#refs#")
	if _, err := refs.OpenAttribute("MATLAB_class"); err == nil {
		t.Fatal("/#refs# must not have a MATLAB_class attribute")
	}

	topic := openTestGroup(t, data, "Topic", "/data/Topic")
	assertMatlabClass(t, topic, "struct", "/data/Topic")

	sig := openTestGroup(t, topic, "sig", "/data/Topic/sig")
	nested := openTestGroup(t, topic, "nested", "/data/Topic/nested")
	count := openTestGroup(t, nested, "count", "/data/Topic/nested/count")
	flag := openTestGroup(t, topic, "flag", "/data/Topic/flag")
	for path, group := range map[string]*hdf5.Group{
		"/data/Topic/sig":          sig,
		"/data/Topic/nested":       nested,
		"/data/Topic/nested/count": count,
		"/data/Topic/flag":         flag,
	} {
		assertMatlabClass(t, group, "struct", path)
	}

	assertDatasetClass(t, sig, "Data", "single", "/data/Topic/sig")
	assertDatasetClass(t, sig, "Timestamp", "double", "/data/Topic/sig")
	assertDatasetClass(t, count, "Data", "int32", "/data/Topic/nested/count")
	assertDatasetClass(t, count, "Timestamp", "double", "/data/Topic/nested/count")
	assertDatasetClass(t, flag, "Data", "logical", "/data/Topic/flag")
	assertDatasetClass(t, flag, "Timestamp", "double", "/data/Topic/flag")

	sigData := openTestDataset(t, sig, "Data", "/data/Topic/sig/Data")
	sigTime := openTestDataset(t, sig, "Timestamp", "/data/Topic/sig/Timestamp")
	if got := readTestFloat32(t, sigData); !slices.Equal(got, []float32{1.5, 2.5, 3.5, 4.5, 5.5}) {
		t.Fatalf("sig Data = %v", got)
	}
	assertRowsAndTimestamps(t, sigData, sigTime, 5, []float64{0, 0.1, 0.2, 0.3, 0.4})

	countData := openTestDataset(t, count, "Data", "/data/Topic/nested/count/Data")
	countTime := openTestDataset(t, count, "Timestamp", "/data/Topic/nested/count/Timestamp")
	if got := readTestInt32(t, countData); !slices.Equal(got, []int32{1, 2, 3, 4, 5}) {
		t.Fatalf("count Data = %v", got)
	}
	assertRowsAndTimestamps(t, countData, countTime, 5, []float64{0, 0.1, 0.2, 0.3, 0.4})

	flagData := openTestDataset(t, flag, "Data", "/data/Topic/flag/Data")
	flagTime := openTestDataset(t, flag, "Timestamp", "/data/Topic/flag/Timestamp")
	if got := readTestUint8(t, flagData); !slices.Equal(got, []uint8{1, 0, 1, 0, 1}) {
		t.Fatalf("flag Data = %v", got)
	}
	assertRowsAndTimestamps(t, flagData, flagTime, 5, []float64{0, 0.1, 0.2, 0.3, 0.4})
	if got := readTestIntAttribute(t, flagData, "MATLAB_int_decode"); got != 1 {
		t.Fatalf("flag MATLAB_int_decode = %d, want 1", got)
	}
}

func TestSupportedValues(t *testing.T) {
	for _, value := range []interface{}{
		float32(0), float64(0),
		int8(0), int16(0), int32(0), int64(0),
		uint8(0), uint16(0), uint32(0), uint64(0),
		true,
	} {
		if !IsSupportedValue(value) {
			t.Fatalf("IsSupportedValue(%T) = false, want true", value)
		}
	}

	for _, value := range []interface{}{"idle", []byte{1}, nil} {
		if IsSupportedValue(value) {
			t.Fatalf("IsSupportedValue(%T) = true, want false", value)
		}
	}
}

func TestWriterRejectsTypeChangeBetweenChunks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "change.mat")

	writer, err := NewWriter(path)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	first := map[string]map[string]interface{}{
		"Topic": {"sig": newFloat32Column([]float32{1}, []float64{0})},
	}
	if err := writer.WriteChunk(first); err != nil {
		t.Fatalf("WriteChunk: %v", err)
	}

	second := map[string]map[string]interface{}{
		"Topic": {"sig": newFloat64Column([]float64{2}, []float64{0.1})},
	}
	if err := writer.WriteChunk(second); err == nil {
		t.Fatal("WriteChunk accepted a column whose type changed between chunks")
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"Topic":        "Topic",
		"nested.value": "nested_value",
		"_private":     "x_private",
		"0speed":       "x0speed",
		"a-b":          "a_b",
	}
	for input, want := range cases {
		got, err := sanitizeName(input)
		if err != nil {
			t.Fatalf("sanitizeName(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("sanitizeName(%q) = %q, want %q", input, got, want)
		}
	}

	if _, err := sanitizeName(""); err == nil {
		t.Fatal("sanitizeName accepted an empty name")
	}
}

func newFloat32Column(values []float32, timestamps []float64) *Column {
	column := NewColumn(values[0])
	for i, value := range values {
		column.Append(value, timestamps[i])
	}
	return column
}

func newFloat64Column(values []float64, timestamps []float64) *Column {
	column := NewColumn(values[0])
	for i, value := range values {
		column.Append(value, timestamps[i])
	}
	return column
}

func newInt32Column(values []int32, timestamps []float64) *Column {
	column := NewColumn(values[0])
	for i, value := range values {
		column.Append(value, timestamps[i])
	}
	return column
}

func newBoolColumn(values []bool, timestamps []float64) *Column {
	column := NewColumn(values[0])
	for i, value := range values {
		column.Append(value, timestamps[i])
	}
	return column
}

type testGroupOpener interface {
	OpenGroup(name string) (*hdf5.Group, error)
}

func openTestGroup(t *testing.T, parent testGroupOpener, name, path string) *hdf5.Group {
	t.Helper()

	group, err := parent.OpenGroup(name)
	if err != nil {
		t.Fatalf("%s: open group: %v", path, err)
	}
	t.Cleanup(func() { group.Close() })
	return group
}

func openTestDataset(t *testing.T, group *hdf5.Group, name, path string) *hdf5.Dataset {
	t.Helper()

	dataset, err := group.OpenDataset(name)
	if err != nil {
		t.Fatalf("%s: open dataset: %v", path, err)
	}
	t.Cleanup(func() { dataset.Close() })
	return dataset
}

type testAttributeOpener interface {
	OpenAttribute(name string) (*hdf5.Attribute, error)
}

func assertMatlabClass(t *testing.T, object testAttributeOpener, want, path string) {
	t.Helper()

	attr, err := object.OpenAttribute("MATLAB_class")
	if err != nil {
		t.Fatalf("%s: open MATLAB_class: %v", path, err)
	}
	defer attr.Close()

	dtype := &hdf5.Datatype{Identifier: attr.GetType()}
	defer dtype.Close()

	var raw [64]byte
	if err := attr.Read(&raw, dtype); err != nil {
		t.Fatalf("%s: read MATLAB_class: %v", path, err)
	}
	if got := string(raw[:dtype.Size()]); got != want {
		t.Fatalf("%s: MATLAB_class = %q, want %q", path, got, want)
	}
}

func assertDatasetClass(t *testing.T, group *hdf5.Group, name, want, path string) {
	t.Helper()

	dataset := openTestDataset(t, group, name, path+"/"+name)
	assertMatlabClass(t, dataset, want, path+"/"+name)
}

func testDatasetDims(t *testing.T, dataset *hdf5.Dataset) []uint {
	t.Helper()

	space := dataset.Space()
	defer space.Close()

	dims, _, err := space.SimpleExtentDims()
	if err != nil {
		t.Fatalf("dataset dims: %v", err)
	}
	return dims
}

func assertRowsAndTimestamps(t *testing.T, data, timestamp *hdf5.Dataset, wantRows int, wantTimes []float64) {
	t.Helper()

	dims := testDatasetDims(t, data)
	if len(dims) == 0 || dims[0] != uint(wantRows) {
		t.Fatalf("Data rows = %v, want first dimension %d", dims, wantRows)
	}
	timeDims := testDatasetDims(t, timestamp)
	if len(timeDims) == 0 || timeDims[0] != uint(wantRows) {
		t.Fatalf("Timestamp rows = %v, want first dimension %d", timeDims, wantRows)
	}
	if got := readTestFloat64(t, timestamp); !slices.Equal(got, wantTimes) {
		t.Fatalf("Timestamp = %v, want %v", got, wantTimes)
	}
}

func readTestFloat32(t *testing.T, dataset *hdf5.Dataset) []float32 {
	t.Helper()

	dims := testDatasetDims(t, dataset)
	values := make([]float32, dims[0])
	if err := dataset.Read(&values); err != nil {
		t.Fatalf("read float32: %v", err)
	}
	return values
}

func readTestFloat64(t *testing.T, dataset *hdf5.Dataset) []float64 {
	t.Helper()

	dims := testDatasetDims(t, dataset)
	values := make([]float64, dims[0])
	if err := dataset.Read(&values); err != nil {
		t.Fatalf("read float64: %v", err)
	}
	return values
}

func readTestInt32(t *testing.T, dataset *hdf5.Dataset) []int32 {
	t.Helper()

	dims := testDatasetDims(t, dataset)
	values := make([]int32, dims[0])
	if err := dataset.Read(&values); err != nil {
		t.Fatalf("read int32: %v", err)
	}
	return values
}

func readTestUint8(t *testing.T, dataset *hdf5.Dataset) []uint8 {
	t.Helper()

	dims := testDatasetDims(t, dataset)
	count := 1
	for _, dim := range dims {
		count *= int(dim)
	}
	values := make([]uint8, count)
	if err := dataset.Read(&values); err != nil {
		t.Fatalf("read uint8: %v", err)
	}
	return values
}

func readTestIntAttribute(t *testing.T, dataset *hdf5.Dataset, name string) int32 {
	t.Helper()

	attr, err := dataset.OpenAttribute(name)
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	defer attr.Close()

	dtype, err := hdf5.NewDataTypeFromType(reflect.TypeFor[int32]())
	if err != nil {
		t.Fatalf("int32 datatype: %v", err)
	}
	defer dtype.Close()

	var value int32
	if err := attr.Read(&value, dtype); err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return value
}
