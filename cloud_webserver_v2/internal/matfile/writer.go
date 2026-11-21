package matfile

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"time"
	"unsafe"
)

type elementKind int

// These values are mirrored by enum matfile_element_kind in hdf5.go.
const (
	kindFloat32 elementKind = iota
	kindFloat64
	kindInt8
	kindInt16
	kindInt32
	kindInt64
	kindUint8
	kindUint16
	kindUint32
	kindUint64
	kindBool
)

func (k elementKind) matlabClass() string {
	switch k {
	case kindFloat32:
		return "single"
	case kindFloat64:
		return "double"
	case kindInt8:
		return "int8"
	case kindInt16:
		return "int16"
	case kindInt32:
		return "int32"
	case kindInt64:
		return "int64"
	case kindUint8:
		return "uint8"
	case kindUint16:
		return "uint16"
	case kindUint32:
		return "uint32"
	case kindUint64:
		return "uint64"
	case kindBool:
		return "logical"
	default:
		return "uint8"
	}
}

const hdf5Unlimited = ^uint64(0)

type signalState struct {
	data      int64
	timestamp int64
	rows      int
	kind      elementKind
}

type Writer struct {
	file     int64
	groups   []int64
	groupIDs map[string]int64
	datasets []int64
	signals  map[string]*signalState
	err      error
	closed   bool
}

func NewWriter(filename string) (*Writer, error) {
	fileID, err := h5CreateFile(filename)
	if err != nil {
		return nil, fmt.Errorf("create MAT file %s: %w", filename, err)
	}

	writer := &Writer{
		file:     fileID,
		groupIDs: make(map[string]int64),
		signals:  make(map[string]*signalState),
	}

	fail := func(err error) (*Writer, error) {
		_ = writer.Close()
		_ = os.Remove(filename)
		return nil, err
	}

	if err := writeMatHeader(filename); err != nil {
		return fail(fmt.Errorf("write MAT header: %w", err))
	}

	if _, err := writer.ensureGroup(fileID, "data", "/data"); err != nil {
		return fail(err)
	}

	refsID, err := h5CreateGroup(fileID, "#refs#")
	if err != nil {
		return fail(fmt.Errorf("create #refs# group: %w", err))
	}
	writer.groups = append(writer.groups, refsID)

	return writer, nil
}

// writeMatHeader writes the 512-byte MATLAB v7.3 user block that precedes the
// HDF5 superblock.
func writeMatHeader(filename string) error {
	header := make([]byte, 512)

	text := fmt.Sprintf("MATLAB 7.3 MAT-file, Platform: GLNXA64, Created on: %s HDF5 schema 1.00 .",
		time.Now().Format("Mon Jan _2 15:04:05 2006"))
	written := copy(header[:116], text)
	for i := written; i < 116; i++ {
		header[i] = ' '
	}
	for i := 116; i < 124; i++ {
		header[i] = ' '
	}
	// Bytes 116-123 are the subsystem offset, 124-125 the MAT version, and
	// 126-127 the endian marker, as MATLAB writes them in the MAT v5 header.
	binary.LittleEndian.PutUint16(header[124:126], 0x0200)
	binary.LittleEndian.PutUint16(header[126:128], 0x4d49)

	file, err := os.OpenFile(filename, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteAt(header, 0)
	return err
}

func (w *Writer) WriteChunk(signals map[string]map[string]interface{}) error {
	if w.closed {
		return errors.New("matfile: writer is closed")
	}
	if w.err != nil {
		return w.err
	}
	if err := w.writeChunk(signals); err != nil {
		w.err = err
		return err
	}
	return nil
}

func (w *Writer) writeChunk(signals map[string]map[string]interface{}) error {
	root := w.groupIDs["/data"]

	for topic, values := range signals {
		name, err := sanitizeName(topic)
		if err != nil {
			return fmt.Errorf("topic %q: %w", topic, err)
		}

		path := "/data/" + name
		group, err := w.ensureGroup(root, name, path)
		if err != nil {
			return err
		}
		if err := w.writeValues(group, path, values); err != nil {
			return err
		}
	}

	return nil
}

func (w *Writer) writeValues(group int64, path string, values map[string]interface{}) error {
	for key, value := range values {
		name, err := sanitizeName(key)
		if err != nil {
			return fmt.Errorf("signal %q: %w", key, err)
		}

		childPath := path + "/" + name
		child, err := w.ensureGroup(group, name, childPath)
		if err != nil {
			return err
		}

		switch v := value.(type) {
		case map[string]interface{}:
			if err := w.writeValues(child, childPath, v); err != nil {
				return err
			}
		case *Column:
			if err := w.writeColumn(child, childPath, v); err != nil {
				return err
			}
		default:
			return fmt.Errorf("signal %s has unsupported value type %T", childPath, value)
		}
	}

	return nil
}

func (w *Writer) ensureGroup(parent int64, name, path string) (int64, error) {
	if id, ok := w.groupIDs[path]; ok {
		return id, nil
	}

	id, err := h5CreateGroup(parent, name)
	if err != nil {
		return 0, fmt.Errorf("create group %s: %w", path, err)
	}
	if err := h5SetClass(id, "struct"); err != nil {
		_ = h5CloseGroup(id)
		return 0, fmt.Errorf("group %s: %w", path, err)
	}

	w.groupIDs[path] = id
	w.groups = append(w.groups, id)
	return id, nil
}

func (w *Writer) writeColumn(group int64, path string, column *Column) error {
	if column.err != nil {
		return fmt.Errorf("could not encode values at %s: %w", path, column.err)
	}
	if column.Len() == 0 {
		return nil
	}

	state, exists := w.signals[path]
	if !exists {
		state = &signalState{kind: column.kind}
		if err := w.createSignal(group, path, column, state); err != nil {
			return err
		}
		w.signals[path] = state
		return nil
	}

	if state.kind != column.kind {
		return fmt.Errorf("signal %s changed data type between chunks (was %v, now %v)", path, state.kind, column.kind)
	}

	return w.appendSignal(state, path, column)
}

func (w *Writer) createSignal(group int64, path string, column *Column, state *signalState) error {
	rows := uint64(column.Len())
	chunkRows := min(rows, maxTableChunkRows)

	dataID, err := h5CreateDataset(group, "Data", int(column.kind), rows, chunkRows)
	if err != nil {
		return fmt.Errorf("%s/Data: %w", path, err)
	}
	w.datasets = append(w.datasets, dataID)
	if err := h5SetClass(dataID, column.kind.matlabClass()); err != nil {
		return fmt.Errorf("%s/Data: %w", path, err)
	}
	if column.kind == kindBool {
		if err := h5SetIntDecode(dataID, 1); err != nil {
			return fmt.Errorf("%s/Data: %w", path, err)
		}
	}

	timeID, err := h5CreateDataset(group, "Timestamp", int(kindFloat64), rows, chunkRows)
	if err != nil {
		return fmt.Errorf("%s/Timestamp: %w", path, err)
	}
	w.datasets = append(w.datasets, timeID)
	if err := h5SetClass(timeID, "double"); err != nil {
		return fmt.Errorf("%s/Timestamp: %w", path, err)
	}

	state.data = dataID
	state.timestamp = timeID
	state.rows = column.Len()

	return w.writeRows(state, path, 0, column)
}

func (w *Writer) appendSignal(state *signalState, path string, column *Column) error {
	offset := state.rows
	newRows := state.rows + column.Len()

	if err := h5ExtendDataset(state.data, uint64(newRows)); err != nil {
		return fmt.Errorf("%s/Data: %w", path, err)
	}
	if err := h5ExtendDataset(state.timestamp, uint64(newRows)); err != nil {
		return fmt.Errorf("%s/Timestamp: %w", path, err)
	}

	if err := w.writeRows(state, path, offset, column); err != nil {
		return err
	}

	state.rows = newRows
	return nil
}

func (w *Writer) writeRows(state *signalState, path string, offset int, column *Column) error {
	count := uint64(column.Len())

	if err := h5WriteDataset(state.data, int(state.kind), uint64(offset), count, unsafe.Pointer(&column.data[0])); err != nil {
		return fmt.Errorf("%s/Data: %w", path, err)
	}
	if err := h5WriteDataset(state.timestamp, int(kindFloat64), uint64(offset), count, unsafe.Pointer(&column.timestamps[0])); err != nil {
		return fmt.Errorf("%s/Timestamp: %w", path, err)
	}

	return nil
}

func (w *Writer) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true

	var firstErr error
	record := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	for _, id := range w.datasets {
		record(h5CloseDataset(id))
	}
	for i := len(w.groups) - 1; i >= 0; i-- {
		record(h5CloseGroup(w.groups[i]))
	}
	record(h5CloseFile(w.file))

	return firstErr
}

func sanitizeName(name string) (string, error) {
	if name == "" {
		return "", errors.New("empty name")
	}

	sanitized := []byte(name)
	for i, b := range sanitized {
		if !isMatlabNameByte(b) {
			sanitized[i] = '_'
		}
	}
	if !isMatlabNameStart(sanitized[0]) {
		sanitized = append([]byte{'x'}, sanitized...)
	}

	return string(sanitized), nil
}

func isMatlabNameByte(b byte) bool {
	return b == '_' ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9')
}

func isMatlabNameStart(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z')
}
