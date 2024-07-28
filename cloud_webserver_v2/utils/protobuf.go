package utils

import (
	"fmt"

	"github.com/foxglove/mcap/go/mcap"
	"github.com/jhump/protoreflect/desc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

type protobufUtils struct {
	protoDescriptions map[string]*desc.FileDescriptor
}

func NewProtobufUtils() *protobufUtils {
	return &protobufUtils{
		protoDescriptions: make(map[string]*desc.FileDescriptor),
	}
}

func (pb *protobufUtils) loadSchema(schema *mcap.Schema) (*desc.FileDescriptor, error) {
	fdSet := &descriptorpb.FileDescriptorSet{}
	if err := proto.Unmarshal(schema.Data, fdSet); err != nil {
		return nil, fmt.Errorf("failed to parse schema data: %w", err)
	}

	files := make([]*desc.FileDescriptor, len(fdSet.GetFile()))
	for i, fd := range fdSet.GetFile() {
		file, err := desc.CreateFileDescriptor(fd)
		if err != nil {
			return nil, fmt.Errorf("failed to create file descriptor for %s: %w", fd.GetName(), err)
		}
		files[i] = file
	}

	pb.protoDescriptions[schema.Name] = files[0]

	return files[0], nil
}

func (pb *protobufUtils) GetDecodedSchema(schema *mcap.Schema) (*desc.FileDescriptor, error) {
	i, ok := pb.protoDescriptions[schema.Name]
	if ok {
		return i, nil
	}

	return pb.loadSchema(schema)
}
