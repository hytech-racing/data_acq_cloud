package utils

import (
	"fmt"
	"log"
	"reflect"

	"gonum.org/v1/hdf5"
)

type HDF5Writer struct {
	file         *hdf5.File
	rootGroup    *hdf5.Group
	currentChunk int
}

func NewHDF5Writer(filename string) (*HDF5Writer, error) {
	file, err := hdf5.CreateFile(filename, hdf5.F_ACC_TRUNC)
	if err != nil {
		return nil, err
	}

	rootGroup, err := file.CreateGroup("data")
	if err != nil {
		return nil, err
	}

	return &HDF5Writer{
		file:         file,
		rootGroup:    rootGroup,
		currentChunk: 0,
	}, nil
}

func (writer *HDF5Writer) ChunkWrite(signalData map[string]map[string]interface{}) error {
	newChunk, err := writer.file.CreateGroup(fmt.Sprintf("/data/chunk_%d", writer.currentChunk))
	if err != nil {
		return err
	}

	err = writer.exploreAndAddDataset("", newChunk, signalData)
	if err != nil {
		return err
	}

	err = newChunk.Close()
	if err != nil {
		return err
	}

	log.Printf("wrote chunk: %v", writer.currentChunk)
	writer.currentChunk += 1
	return nil
}

func (writer *HDF5Writer) exploreAndAddDataset(path string, chunk *hdf5.Group, data interface{}) error {
	switch data.(type) {
	case map[string]map[string]interface{}:
		castedData := data.(map[string]map[string]interface{})
		for key, val := range castedData {
			err := writer.exploreAndAddDataset(path+key, chunk, val)
			if err != nil {
				return err
			}
		}

	case map[string]interface{}:
		castedData := data.(map[string]interface{})
		for key, val := range castedData {
			err := writer.exploreAndAddDataset(path+"."+key, chunk, val)
			if err != nil {
				return err
			}
		}

	case [][]float64:
		flattenedSlice := FlattenSlice(data.([][]float64))
		dims := []uint{uint(len(flattenedSlice) / 2), 2} // 2 rows: timestamp and value
		dspace, err := hdf5.CreateSimpleDataspace(dims, nil)
		if err != nil {
			return fmt.Errorf("failed to create dataspace for path %s: %v", path, err)
		}
		defer dspace.Close()

		// Create a dataset within the group for the specific signal
		dataset, err := chunk.CreateDataset(path, hdf5.T_NATIVE_DOUBLE, dspace)
		if err != nil {
			return fmt.Errorf("failed to create dataset for %s: %v", path, err)
		}
		defer dataset.Close()

		// Write the data to the dataset
		if err := dataset.Write(&flattenedSlice); err != nil {
			return fmt.Errorf("failed to write data to %s dataset: %v", path, err)
		}
	default:
		log.Printf("unsupported type: %v", reflect.TypeOf(data))
	}

	return nil
}

func FlattenSlice(data [][]float64) []float64 {
	flattened := make([]float64, len(data)*len(data[0]))
	for i, innerList := range data {
		for j, val := range innerList {
			flattened[i*len(data[0])+j] = val
		}
	}

	return flattened
}

func (writer *HDF5Writer) Close() error {
	err := writer.rootGroup.Close()
	if err != nil {
		return fmt.Errorf("could not close rootGroup: %v", err)
	}

	err = writer.file.Close()
	if err != nil {
		return fmt.Errorf("could not close HDF5 file: %v", err)
	}

	return nil
}
