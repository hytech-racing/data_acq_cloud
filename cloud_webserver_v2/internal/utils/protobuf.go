package utils

import (
	"fmt"

	"github.com/foxglove/mcap/go/mcap"
	"github.com/jhump/protoreflect/desc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// MCAP doesn't have a native protobuf wrapper for golang, so we have to
// dynamically decode the schemas and messages ourselves :)

// ProtobufUtils contains descriptions and descriptors which are stored to allow for
// faster and dynamic parsing of protobuf encoded data
type ProtobufUtils struct {
	protoDescriptions  map[string]*desc.FileDescriptor
	protoDescriptorSet *descriptorpb.FileDescriptorSet
}

func NewProtobufUtils() *ProtobufUtils {
	return &ProtobufUtils{
		protoDescriptions:  make(map[string]*desc.FileDescriptor),
		protoDescriptorSet: &descriptorpb.FileDescriptorSet{},
	}
}

func (pb *ProtobufUtils) GetDecodedSchema(schema *mcap.Schema) (*desc.FileDescriptor, error) {
	i, ok := pb.protoDescriptions[schema.Name]
	if ok {
		return i, nil
	}

	return pb.loadSchema(schema)
}

func (pb *ProtobufUtils) loadSchema(schema *mcap.Schema) (*desc.FileDescriptor, error) {
	// We are using this as a cache so we can use the cached descriptors to decode new ones
	fdSet := &pb.protoDescriptorSet
	if err := proto.Unmarshal(schema.Data, *fdSet); err != nil {
		return nil, fmt.Errorf("failed to parse schema data: %w", err)
	}

	fdSetFiles := (*fdSet).GetFile()
	successfulFiles := make([]*desc.FileDescriptor, 0)
	errFiles := make([]*descriptorpb.FileDescriptorProto, 0, len(fdSetFiles))

	// Each MCAP schema can give us all the proto descriptors we need but not necessarily in the order we need them
	// As far as I can tell, it is random depending on how we create the schema upon MCAP generation
	// The way we can get around this is to decode as many schemas we possible, store these, and use them as dependencies
	// to decode the higher level schemas which require them.
	for range len(fdSetFiles) {
		for _, fd := range fdSetFiles {
			if len(successfulFiles) == len(fdSetFiles) {
				break
			}

			if pb.protoDescriptions[fd.GetName()] != nil {
				successfulFiles = append(successfulFiles, pb.protoDescriptions[fd.GetName()])
				continue
			}

			file, err := desc.CreateFileDescriptor(fd, successfulFiles...)
			if err != nil {
				errFiles = append(errFiles, fd)
				continue
			}
			successfulFiles = append(successfulFiles, file)
			pb.protoDescriptions[fd.GetName()] = file

			fdSetFiles = errFiles
		}
	}

	if len(errFiles) != 0 || len(successfulFiles) == 0 {
		return nil, fmt.Errorf("failed to create file descriptors for %v", errFiles)
	}

	// To find the highest level schema file, we need to find the one with the largest number of dependencies
	maxDepLen := -1
	var highestLevelFile *desc.FileDescriptor = nil
	for _, succFile := range successfulFiles {
		depLen := len(succFile.AsFileDescriptorProto().Dependency)
		if depLen > maxDepLen {
			highestLevelFile = succFile
			maxDepLen = depLen
		}
	}

	pb.protoDescriptions[schema.Name] = highestLevelFile
	fdProto := highestLevelFile.AsFileDescriptorProto()
	pb.protoDescriptorSet.File = append(pb.protoDescriptorSet.File, fdProto)

	return highestLevelFile, nil
}
