// Adapted from https://github.com/foxglove/mcap/blob/main/go/cli/mcap/cmd/recover.go

package utils

import (
	"errors"
	"fmt"
	"io"
	"log"

	"github.com/foxglove/mcap/go/cli/mcap/utils"
	"github.com/foxglove/mcap/go/mcap"
)

type RecoverOptions struct {
	DecodeChunk bool
	ChunkSize   int64
	Compression mcap.CompressionFormat
}

func RecoverRun(
	r io.Reader,
	w io.Writer,
	ops *RecoverOptions,
) error {
	log.Println("RUNNING RECOVERY FUNCTION")
	if ops == nil {
		ops = &RecoverOptions{
			DecodeChunk: false,
			ChunkSize:   64,
			Compression: mcap.CompressionNone,
		}
	}

	decodeChunk := ops.DecodeChunk
	mcapWriter, err := mcap.NewWriter(w, &mcap.WriterOptions{
		Chunked:     true,
		ChunkSize:   ops.ChunkSize,
		Compression: ops.Compression,
	})
	if err != nil {
		return err
	}

	info := &mcap.Info{
		Statistics: &mcap.Statistics{
			ChannelMessageCounts: make(map[uint16]uint64),
		},
		Channels: make(map[uint16]*mcap.Channel),
		Schemas:  make(map[uint16]*mcap.Schema),
	}

	defer func() {
		mcapWriter.Statistics.MessageCount += info.Statistics.MessageCount
		for channelID, count := range info.Statistics.ChannelMessageCounts {
			mcapWriter.Statistics.ChannelMessageCounts[channelID] += count
		}

		for _, schema := range info.Schemas {
			mcapWriter.AddSchema(schema)
		}
		for _, channel := range info.Channels {
			mcapWriter.AddChannel(channel)
		}

		err := mcapWriter.Close()
		if err != nil {
			utils.EprintF("failed to close mcap writer: %v\n", err)
			return
		}
		utils.EprintF(
			"Recovered %d messages, %d attachments, and %d metadata records.\n",
			mcapWriter.Statistics.MessageCount,
			mcapWriter.Statistics.AttachmentCount,
			mcapWriter.Statistics.MetadataCount,
		)
	}()

	lexer, err := mcap.NewLexer(r, &mcap.LexerOptions{
		ValidateChunkCRCs: true,
		EmitChunks:        !ops.DecodeChunk,
		EmitInvalidChunks: true,
		AttachmentCallback: func(ar *mcap.AttachmentReader) error {
			err = mcapWriter.WriteAttachment(&mcap.Attachment{
				LogTime:    ar.LogTime,
				CreateTime: ar.CreateTime,
				Name:       ar.Name,
				MediaType:  ar.MediaType,
				DataSize:   ar.DataSize,
				Data:       ar.Data(),
			})
			if err != nil {
				return err
			}
			return nil
		},
	})
	if err != nil {
		return err
	}

	buf := make([]byte, 1024)
	var lastChunk *mcap.Chunk
	var lastIndexes []*mcap.MessageIndex
	var recordsCopy []byte

	for {
		token, data, err := lexer.Next(buf)
		if err != nil {
			if token == mcap.TokenInvalidChunk {
				utils.EprintF("Invalid chunk encountered, skipping: %s\n", err)
				continue
			}
			if lastChunk != nil {
				// Reconstruct message indexes for the last chunk, because it is unclear if the
				// message indexes are complete or not.
				idx, err := utils.UpdateInfoFromChunk(info, lastChunk, nil)
				if err != nil {
					utils.EprintF("Failed to update info from chunk, skipping: %s\n", err)
				} else {
					err = mcapWriter.WriteChunkWithIndexes(lastChunk, idx)
					if err != nil {
						utils.EprintF("Failed to write chunk, skipping: %s\n", err)
					}
				}
			}
			if errors.Is(err, io.EOF) {
				return nil
			}
			var expected *mcap.ErrTruncatedRecord
			if errors.As(err, &expected) {
				utils.Eprintln(expected.Error())
				return nil
			}
			return nil
		}
		if len(data) > len(buf) {
			buf = data
		}

		if token != mcap.TokenMessageIndex {
			if lastChunk != nil {
				lastIndexes, err = utils.UpdateInfoFromChunk(info, lastChunk, lastIndexes)
				if err != nil {
					utils.EprintF("Failed to update info from chunk, skipping: %s\n", err)
				} else {
					err = mcapWriter.WriteChunkWithIndexes(lastChunk, lastIndexes)
					if err != nil {
						utils.EprintF("Failed to write chunk, skipping: %s\n", err)
					}
				}
				lastIndexes = nil
				lastChunk = nil
			}
		}

		switch token {
		case mcap.TokenHeader:
			header, err := mcap.ParseHeader(data)
			if err != nil {
				return err
			}
			if err := mcapWriter.WriteHeader(header); err != nil {
				return err
			}
		case mcap.TokenChunk:
			chunk, err := mcap.ParseChunk(data)

			if decodeChunk {
				idx, err := utils.UpdateInfoFromChunk(info, chunk, nil)
				if err != nil {
					utils.EprintF("Failed to update info from chunk, skipping: %s\n", err)
				} else {
					err = mcapWriter.WriteChunkWithIndexes(chunk, idx)
					if err != nil {
						utils.EprintF("Failed to write chunk, skipping: %s\n", err)
					}
				}
			} else {
				// copy the records, since it is referenced and the buffer will be reused
				if cap(recordsCopy) < len(chunk.Records) {
					recordsCopy = make([]byte, len(chunk.Records))
				} else {
					recordsCopy = recordsCopy[:len(chunk.Records)]
				}
				copy(recordsCopy, chunk.Records)
				lastChunk = chunk
				lastChunk.Records = recordsCopy

				if err != nil {
					return err
				}
			}
		case mcap.TokenMessageIndex:
			if !decodeChunk {
				if lastChunk == nil {
					return fmt.Errorf("got message index but not chunk before it")
				}
				index, err := mcap.ParseMessageIndex(data)
				if err != nil {
					return err
				}
				lastIndexes = append(lastIndexes, index)
			}
		case mcap.TokenMetadata:
			metadata, err := mcap.ParseMetadata(data)
			if err != nil {
				return err
			}
			if err := mcapWriter.WriteMetadata(metadata); err != nil {
				return err
			}
		case mcap.TokenSchema:
			decodeChunk = true // mcap is not chunked
			schema, err := mcap.ParseSchema(data)
			if err != nil {
				return err
			}
			if err := mcapWriter.WriteSchema(schema); err != nil {
				return err
			}
		case mcap.TokenChannel:
			decodeChunk = true // mcap is not chunked
			channel, err := mcap.ParseChannel(data)
			if err != nil {
				return err
			}
			if err := mcapWriter.WriteChannel(channel); err != nil {
				return err
			}
		case mcap.TokenMessage:
			decodeChunk = true // mcap is not chunked
			message, err := mcap.ParseMessage(data)
			if err != nil {
				return err
			}
			if err := mcapWriter.WriteMessage(message); err != nil {
				return err
			}
		case mcap.TokenDataEnd, mcap.TokenFooter:
			// data section is over, either because the file is over or the summary section starts.
			return nil
		case mcap.TokenError:
			return errors.New("received error token but lexer did not return error on Next")
		}
	}
}
