package utils

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"

	"gonum.org/v1/hdf5"
)

// Every row of a signal's HDF5 dataset is a compound {Data, Timestamp}.
// Numeric values sit at the start of a 16 byte Data slot. []byte and string values
// use a fixed-length byte array to avoid variable-length global heap exhaustion in HDF5 1.10.x.
const (
	numericRowSize         = 24
	numericTimestampOffset = 16

	fixedBytesRowSize         = MaxFixedBytesLen + 8
	fixedBytesTimestampOffset = MaxFixedBytesLen
)

// hdf5Unlimited is H5S_UNLIMITED, which lets a dataset dimension grow without a max size
const hdf5Unlimited = ^uint(0)

// HDF5Column holds the rows of one signal in the exact memory layout of its HDF5 compound type,
// so the whole column can be handed to HDF5 in a single write without encoding each row.
// The buffer holds no Go pointers, so the garbage collector never scans its contents.
type HDF5Column struct {
	buf             []byte
	rows            int
	rowSize         int
	timestampOffset int
	fixedBytes      bool

	// dataType is the Go type of the first value, which decides the HDF5 type of the Data field
	dataType reflect.Type

	// err is the first value that could not be encoded. It is returned when the column is written.
	err error
}

// NewHDF5Column creates an empty column whose HDF5 type is chosen from the signal's first value
func NewHDF5Column(firstValue interface{}) *HDF5Column {
	column := &HDF5Column{
		rowSize:         numericRowSize,
		timestampOffset: numericTimestampOffset,
		dataType:        reflect.TypeOf(firstValue),
	}

	switch firstValue.(type) {
	case []byte, string:
		column.fixedBytes = true
		column.rowSize = fixedBytesRowSize
		column.timestampOffset = fixedBytesTimestampOffset
	}

	return column
}

// Append adds a row to the end of the column
func (c *HDF5Column) Append(value interface{}, timestamp float64) {
	start := len(c.buf)
	c.buf = append(c.buf, make([]byte, c.rowSize)...)
	row := c.buf[start:]

	if c.fixedBytes {
		switch v := value.(type) {
		case []byte:
			copy(row[:MaxFixedBytesLen], v)
		case string:
			copy(row[:MaxFixedBytesLen], v)
		}
	} else if err := putNativeValue(row[:c.timestampOffset], value); err != nil && c.err == nil {
		c.err = err
	}

	binary.NativeEndian.PutUint64(row[c.timestampOffset:], math.Float64bits(timestamp))
	c.rows++
}

func (c *HDF5Column) Len() int {
	return c.rows
}

func (c *HDF5Column) LastTimestamp() float64 {
	offset := (c.rows-1)*c.rowSize + c.timestampOffset
	return math.Float64frombits(binary.NativeEndian.Uint64(c.buf[offset:]))
}

// Write creates a chunked dataset at path in group and writes every row of the column into it with one call
func (c *HDF5Column) Write(group *hdf5.Group, path string) error {
	if c.rows == 0 {
		return nil
	}
	if c.err != nil {
		return fmt.Errorf("could not encode values at path %q (type %v): %w", path, c.dataType, c.err)
	}

	dtype, err := c.compoundType()
	if err != nil {
		return fmt.Errorf("could not create HDF5 type at path %q (type %v): %w", path, c.dataType, err)
	}
	defer dtype.Close()

	dspace, err := hdf5.CreateSimpleDataspace([]uint{uint(c.rows)}, []uint{hdf5Unlimited})
	if err != nil {
		return fmt.Errorf("could not create dataspace at path %q: %w", path, err)
	}
	defer dspace.Close()

	dcpl, err := hdf5.NewPropList(hdf5.P_DATASET_CREATE)
	if err != nil {
		return fmt.Errorf("could not create dataset properties at path %q: %w", path, err)
	}
	defer dcpl.Close()

	// Small signals use their row count so they don't allocate a mostly empty chunk
	if err := dcpl.SetChunk([]uint{uint(min(c.rows, MaxTableChunkRows))}); err != nil {
		return fmt.Errorf("could not set chunk size at path %q: %w", path, err)
	}

	dataset, err := group.CreateDatasetWith(path, dtype, dspace, dcpl)
	if err != nil {
		return fmt.Errorf("could not create dataset at path %q (type %v): %w", path, c.dataType, err)
	}
	defer dataset.Close()

	// buf already has the compound type's layout, so HDF5 reads it as is
	if err := dataset.Write(&c.buf); err != nil {
		return fmt.Errorf("could not write dataset at path %q (type %v): %w", path, c.dataType, err)
	}

	return nil
}

// compoundType builds the HDF5 type that matches the column's row layout
func (c *HDF5Column) compoundType() (*hdf5.Datatype, error) {
	compound, err := hdf5.NewCompoundType(c.rowSize)
	if err != nil {
		return nil, err
	}

	var dataField *hdf5.Datatype
	if c.fixedBytes {
		elemType, err := hdf5.NewDataTypeFromType(reflect.TypeFor[uint8]())
		if err != nil {
			return nil, err
		}
		defer elemType.Close()

		arrayType, err := hdf5.NewArrayType(elemType, []int{MaxFixedBytesLen})
		if err != nil {
			return nil, err
		}
		dataField = &arrayType.Datatype
	} else {
		if c.dataType == nil {
			return nil, errors.New("signal has no value type")
		}
		dataField, err = hdf5.NewDataTypeFromType(c.dataType)
		if err != nil {
			return nil, err
		}
	}
	defer dataField.Close()

	if err := compound.Insert("Data", 0, dataField); err != nil {
		return nil, err
	}

	timestampField, err := hdf5.NewDataTypeFromType(reflect.TypeFor[float64]())
	if err != nil {
		return nil, err
	}
	defer timestampField.Close()

	if err := compound.Insert("Timestamp", c.timestampOffset, timestampField); err != nil {
		return nil, err
	}

	return &compound.Datatype, nil
}

// putNativeValue writes a numeric or bool value into dst in native byte order, the way C lays it out in memory
func putNativeValue(dst []byte, value interface{}) error {
	switch v := value.(type) {
	case float32:
		binary.NativeEndian.PutUint32(dst, math.Float32bits(v))
	case float64:
		binary.NativeEndian.PutUint64(dst, math.Float64bits(v))
	case int32:
		binary.NativeEndian.PutUint32(dst, uint32(v))
	case uint32:
		binary.NativeEndian.PutUint32(dst, v)
	case int64:
		binary.NativeEndian.PutUint64(dst, uint64(v))
	case uint64:
		binary.NativeEndian.PutUint64(dst, v)
	case bool:
		if v {
			dst[0] = 1
		}
	case int8:
		dst[0] = byte(v)
	case uint8:
		dst[0] = v
	case int16:
		binary.NativeEndian.PutUint16(dst, uint16(v))
	case uint16:
		binary.NativeEndian.PutUint16(dst, v)
	default:
		return fmt.Errorf("unsupported value type %T", value)
	}

	return nil
}
