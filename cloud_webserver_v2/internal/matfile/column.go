package matfile

import (
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
)

const maxTableChunkRows = 4096

// Column holds one signal's rows in the compact layout its HDF5 dataset uses.
// Data is raw values in host byte order.
type Column struct {
	data       []byte
	timestamps []float64
	elemSize   int
	kind       elementKind
	dataType   reflect.Type
	err        error
}

func IsSupportedValue(value interface{}) bool {
	switch value.(type) {
	case float32, float64,
		int8, int16, int32, int64,
		uint8, uint16, uint32, uint64,
		bool:
		return true
	default:
		return false
	}
}

func NewColumn(firstValue interface{}) *Column {
	column := &Column{dataType: reflect.TypeOf(firstValue)}

	switch firstValue.(type) {
	case float32:
		column.kind = kindFloat32
		column.elemSize = 4
	case float64:
		column.kind = kindFloat64
		column.elemSize = 8
	case int8:
		column.kind = kindInt8
		column.elemSize = 1
	case int16:
		column.kind = kindInt16
		column.elemSize = 2
	case int32:
		column.kind = kindInt32
		column.elemSize = 4
	case int64:
		column.kind = kindInt64
		column.elemSize = 8
	case uint8:
		column.kind = kindUint8
		column.elemSize = 1
	case uint16:
		column.kind = kindUint16
		column.elemSize = 2
	case uint32:
		column.kind = kindUint32
		column.elemSize = 4
	case uint64:
		column.kind = kindUint64
		column.elemSize = 8
	case bool:
		column.kind = kindBool
		column.elemSize = 1
	default:
		column.elemSize = 1
		column.err = fmt.Errorf("unsupported value type %T", firstValue)
	}

	return column
}

func (c *Column) Append(value interface{}, timestamp float64) {
	start := len(c.data)
	c.data = append(c.data, make([]byte, c.elemSize)...)
	row := c.data[start:]

	if reflect.TypeOf(value) != c.dataType {
		c.recordError(fmt.Errorf("cannot store %T in a %v column", value, c.dataType))
	} else if err := putNativeValue(row, value); err != nil {
		c.recordError(err)
	}

	c.timestamps = append(c.timestamps, timestamp)
}

func (c *Column) recordError(err error) {
	if c.err == nil {
		c.err = err
	}
}

func (c *Column) Len() int {
	return len(c.timestamps)
}

func (c *Column) LastTimestamp() float64 {
	if len(c.timestamps) == 0 {
		return 0
	}
	return c.timestamps[len(c.timestamps)-1]
}

// putNativeValue writes value into dst in host byte order, which is the memory
// type HDF5 is told the buffer uses.
func putNativeValue(dst []byte, value interface{}) error {
	switch v := value.(type) {
	case float32:
		binary.NativeEndian.PutUint32(dst, math.Float32bits(v))
	case float64:
		binary.NativeEndian.PutUint64(dst, math.Float64bits(v))
	case int8:
		dst[0] = byte(v)
	case int16:
		binary.NativeEndian.PutUint16(dst, uint16(v))
	case int32:
		binary.NativeEndian.PutUint32(dst, uint32(v))
	case int64:
		binary.NativeEndian.PutUint64(dst, uint64(v))
	case uint8:
		dst[0] = v
	case uint16:
		binary.NativeEndian.PutUint16(dst, v)
	case uint32:
		binary.NativeEndian.PutUint32(dst, v)
	case uint64:
		binary.NativeEndian.PutUint64(dst, v)
	case bool:
		if v {
			dst[0] = 1
		} else {
			dst[0] = 0
		}
	default:
		return fmt.Errorf("unsupported value type %T", value)
	}

	return nil
}
