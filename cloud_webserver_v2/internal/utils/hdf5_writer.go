package utils

import (
	"fmt"
	"log"
	"reflect"

	"gonum.org/v1/hdf5"
)

// Wrapper to store our message data and timestamps
type HDF5WrapperMessage struct {
	Data      interface{} `hdf5:"Message"`
	Timestamp float64     `hdf5:"Timestamp"`
}

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

// ChunkWrite creates a new group in the HDF5 file and writes all data in the signalData map into it
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

// exploreAndAddDataset recursively explores the data interface and performs a deep copy of the data
// interface and writes the information into its respective dataset in the current HDF5 Group
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

	case []*HDF5WrapperMessage:
		// Create our own DataType based on what data we have
		dtype, err := CreateHDF5DataType(data.([]*HDF5WrapperMessage)) // is it str, char, slice etc?
		if err != nil {
			return err
		}

		// write table to chunk
		table, err := chunk.CreateTable(path, dtype, 10, 0)

		if err != nil {
			return err
		}
		defer table.Close()

		//Append data to table
		for i := 0; i != len(data.([]*HDF5WrapperMessage)); i++ {
			if err = table.Append(data.([]*HDF5WrapperMessage)[i]); err != nil {
				return err
			}
		}

	// Leaving this open for interpolatedMat subscriber for now but we should really take this out tbh
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

// FlattenSlice flattens a 2D slice to 1D
func FlattenSlice(data [][]float64) []float64 {
	flattened := make([]float64, len(data)*len(data[0]))
	for i, innerList := range data {
		for j, val := range innerList {
			flattened[i*len(data[0])+j] = val
		}
	}

	return flattened
}

// CreateDataType func Creates DataType based on the given message
func CreateHDF5DataType(data []*HDF5WrapperMessage) (*hdf5.Datatype, error) {
	var dtype *hdf5.Datatype

	// Find size of DataType
	t := reflect.TypeOf(HDF5WrapperMessage{})
	sz := int(t.Size())

	// Since our HDF5WrapperMessageData type is a struct, we want to add fields (=> Compound Type)
	cdt, err := hdf5.NewCompoundType(sz)
	if err != nil {
		return nil, err
	}

	// Create Message field
	data_field_dt, err := hdf5.NewDataTypeFromType(reflect.TypeOf(data[0].Data))
	if err != nil {
		return nil, err
	}
	err = cdt.Insert("Data", 0, data_field_dt)
	if err != nil {
		return nil, err
	}

	// Create Timestamp field
	timestamp_field_dt, err := hdf5.NewDataTypeFromType(reflect.TypeFor[float64]())
	if err != nil {
		return nil, err
	}
	err = cdt.Insert("Timestamp", 16, timestamp_field_dt)
	if err != nil {
		return nil, err
	}
	dtype = &cdt.Datatype
	return dtype, nil
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
