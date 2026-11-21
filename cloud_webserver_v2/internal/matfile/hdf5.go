package matfile

/*
#cgo LDFLAGS: -lhdf5 -lhdf5_hl
#cgo darwin CFLAGS: -I/usr/local/include
#cgo darwin LDFLAGS: -L/usr/local/lib
#cgo linux,!arm64 CFLAGS: -I/usr/local/include -I/usr/lib/x86_64-linux-gnu/hdf5/serial/include
#cgo linux,!arm64 LDFLAGS: -L/usr/local/lib -L/usr/lib/x86_64-linux-gnu/hdf5/serial/
#cgo linux,arm64 CFLAGS: -I/usr/local/include -I/usr/lib/aarch64-linux-gnu/hdf5/serial/include
#cgo linux,arm64 LDFLAGS: -L/usr/local/lib -L/usr/lib/aarch64-linux-gnu/hdf5/serial/

#include <hdf5.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

// matfile_element_kind values mirror the elementKind constants in writer.go.
enum matfile_element_kind {
	MATFILE_FLOAT32 = 0,
	MATFILE_FLOAT64,
	MATFILE_INT8,
	MATFILE_INT16,
	MATFILE_INT32,
	MATFILE_INT64,
	MATFILE_UINT8,
	MATFILE_UINT16,
	MATFILE_UINT32,
	MATFILE_UINT64,
	MATFILE_BOOL
};

static hid_t matfile_file_type(int kind) {
	switch (kind) {
	case MATFILE_FLOAT32: return H5T_IEEE_F32LE;
	case MATFILE_FLOAT64: return H5T_IEEE_F64LE;
	case MATFILE_INT8: return H5T_STD_I8LE;
	case MATFILE_INT16: return H5T_STD_I16LE;
	case MATFILE_INT32: return H5T_STD_I32LE;
	case MATFILE_INT64: return H5T_STD_I64LE;
	case MATFILE_UINT8: return H5T_STD_U8LE;
	case MATFILE_UINT16: return H5T_STD_U16LE;
	case MATFILE_UINT32: return H5T_STD_U32LE;
	case MATFILE_UINT64: return H5T_STD_U64LE;
	case MATFILE_BOOL: return H5T_STD_U8LE;
	}
	return H5T_STD_U8LE;
}

static hid_t matfile_mem_type(int kind) {
	switch (kind) {
	case MATFILE_FLOAT32: return H5T_NATIVE_FLOAT;
	case MATFILE_FLOAT64: return H5T_NATIVE_DOUBLE;
	case MATFILE_INT8: return H5T_NATIVE_INT8;
	case MATFILE_INT16: return H5T_NATIVE_INT16;
	case MATFILE_INT32: return H5T_NATIVE_INT32;
	case MATFILE_INT64: return H5T_NATIVE_INT64;
	case MATFILE_UINT8: return H5T_NATIVE_UINT8;
	case MATFILE_UINT16: return H5T_NATIVE_UINT16;
	case MATFILE_UINT32: return H5T_NATIVE_UINT32;
	case MATFILE_UINT64: return H5T_NATIVE_UINT64;
	case MATFILE_BOOL: return H5T_NATIVE_UINT8;
	}
	return H5T_NATIVE_UINT8;
}

static hid_t matfile_create_file(const char *path) {
	hid_t fapl = H5Pcreate(H5P_FILE_ACCESS);
	if (fapl < 0) return -1;
	if (H5Pset_libver_bounds(fapl, H5F_LIBVER_V18, H5F_LIBVER_V18) < 0) {
		H5Pclose(fapl);
		return -1;
	}
	hid_t fcpl = H5Pcreate(H5P_FILE_CREATE);
	if (fcpl < 0) {
		H5Pclose(fapl);
		return -1;
	}
	if (H5Pset_userblock(fcpl, 512) < 0) {
		H5Pclose(fcpl);
		H5Pclose(fapl);
		return -1;
	}
	hid_t file = H5Fcreate(path, H5F_ACC_TRUNC, fcpl, fapl);
	H5Pclose(fcpl);
	H5Pclose(fapl);
	return file;
}

static hid_t matfile_create_group(hid_t loc, const char *name) {
	return H5Gcreate2(loc, name, H5P_DEFAULT, H5P_DEFAULT, H5P_DEFAULT);
}

static int matfile_set_class(hid_t obj, const char *class) {
	hid_t space = H5Screate(H5S_SCALAR);
	hid_t type = H5Tcopy(H5T_C_S1);
	H5Tset_size(type, strlen(class));
	H5Tset_strpad(type, H5T_STR_NULLTERM);
	hid_t attr = H5Acreate2(obj, "MATLAB_class", type, space, H5P_DEFAULT, H5P_DEFAULT);
	int rc = -1;
	if (attr >= 0) {
		rc = H5Awrite(attr, type, class);
		H5Aclose(attr);
	}
	H5Tclose(type);
	H5Sclose(space);
	return rc;
}

static int matfile_set_int_decode(hid_t obj, int32_t value) {
	hid_t space = H5Screate(H5S_SCALAR);
	hid_t attr = H5Acreate2(obj, "MATLAB_int_decode", H5T_STD_I32LE, space, H5P_DEFAULT, H5P_DEFAULT);
	int rc = -1;
	if (attr >= 0) {
		rc = H5Awrite(attr, H5T_NATIVE_INT32, &value);
		H5Aclose(attr);
	}
	H5Sclose(space);
	return rc;
}

static hid_t matfile_create_dataset(hid_t group, const char *name, int kind, hsize_t rows, hsize_t chunk_rows) {
	hid_t type = matfile_file_type(kind);
	hsize_t max_rows = H5S_UNLIMITED;
	hid_t space = H5Screate_simple(1, &rows, &max_rows);
	if (space < 0) return -1;
	hid_t dcpl = H5Pcreate(H5P_DATASET_CREATE);
	if (dcpl < 0) {
		H5Sclose(space);
		return -1;
	}
	hsize_t chunks[1] = {chunk_rows};
	H5Pset_chunk(dcpl, 1, chunks);
	hid_t dataset = H5Dcreate2(group, name, type, space, H5P_DEFAULT, dcpl, H5P_DEFAULT);
	H5Pclose(dcpl);
	H5Sclose(space);
	return dataset;
}

static int matfile_extend(hid_t dataset, hsize_t rows) {
	hsize_t dims[1] = {rows};
	return H5Dset_extent(dataset, dims);
}

static int matfile_write(hid_t dataset, int kind, hsize_t offset, hsize_t count, const void *buf) {
	hid_t file_space = H5Dget_space(dataset);
	if (file_space < 0) return -1;
	hsize_t start[1] = {offset};
	hsize_t length[1] = {count};
	if (H5Sselect_hyperslab(file_space, H5S_SELECT_SET, start, NULL, length, NULL) < 0) {
		H5Sclose(file_space);
		return -1;
	}
	hid_t mem_space = H5Screate_simple(1, length, NULL);
	if (mem_space < 0) {
		H5Sclose(file_space);
		return -1;
	}
	int rc = H5Dwrite(dataset, matfile_mem_type(kind), mem_space, file_space, H5P_DEFAULT, buf);
	H5Sclose(mem_space);
	H5Sclose(file_space);
	return rc;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func h5CreateFile(path string) (int64, error) {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	id := C.matfile_create_file(cPath)
	if id < 0 {
		return 0, fmt.Errorf("create hdf5 file %q", path)
	}
	return int64(id), nil
}

func h5CreateGroup(parent int64, name string) (int64, error) {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	id := C.matfile_create_group(C.hid_t(parent), cName)
	if id < 0 {
		return 0, fmt.Errorf("create group %q", name)
	}
	return int64(id), nil
}

func h5SetClass(obj int64, class string) error {
	cClass := C.CString(class)
	defer C.free(unsafe.Pointer(cClass))

	if C.matfile_set_class(C.hid_t(obj), cClass) < 0 {
		return fmt.Errorf("set MATLAB_class %q", class)
	}
	return nil
}

func h5SetIntDecode(obj int64, value int32) error {
	if C.matfile_set_int_decode(C.hid_t(obj), C.int32_t(value)) < 0 {
		return fmt.Errorf("set MATLAB_int_decode %d", value)
	}
	return nil
}

func h5CreateDataset(group int64, name string, kind int, rows, chunkRows uint64) (int64, error) {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	id := C.matfile_create_dataset(C.hid_t(group), cName, C.int(kind), C.hsize_t(rows), C.hsize_t(chunkRows))
	if id < 0 {
		return 0, fmt.Errorf("create dataset %q", name)
	}
	return int64(id), nil
}

func h5ExtendDataset(dataset int64, rows uint64) error {
	if C.matfile_extend(C.hid_t(dataset), C.hsize_t(rows)) < 0 {
		return fmt.Errorf("extend dataset")
	}
	return nil
}

func h5WriteDataset(dataset int64, kind int, offset, count uint64, buf unsafe.Pointer) error {
	if C.matfile_write(C.hid_t(dataset), C.int(kind), C.hsize_t(offset), C.hsize_t(count), buf) < 0 {
		return fmt.Errorf("write dataset")
	}
	return nil
}

func h5CloseDataset(id int64) error {
	if C.H5Dclose(C.hid_t(id)) < 0 {
		return fmt.Errorf("close dataset")
	}
	return nil
}

func h5CloseGroup(id int64) error {
	if C.H5Gclose(C.hid_t(id)) < 0 {
		return fmt.Errorf("close group")
	}
	return nil
}

func h5CloseFile(id int64) error {
	if C.H5Fclose(C.hid_t(id)) < 0 {
		return fmt.Errorf("close hdf5 file")
	}
	return nil
}
