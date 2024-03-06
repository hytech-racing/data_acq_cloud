import os
import mcap.exceptions
from mcap.decoder import DecoderFactory
from mcap_protobuf.decoder import DecoderFactory
from mcap.reader import make_reader
from mcap_protobuf.writer import Writer


class MCAPHandler():
    def __init__(self, mcap_file_path):
        self.mcap_file_path = mcap_file_path
        self.accepted_tire_topics = ["lf_ttpms_1", "rf_ttpms_1", "lr_ttpms_1", "rr_ttpms_1"]
        self.avg_pressures = {"lf_ttpms_1": 0, "rf_ttpms_1": 0, "lr_ttpms_1": 0, "rr_ttpms_1": 0}
        self.pressure_count = {"lf_ttpms_1": 0, "rf_ttpms_1": 0, "lr_ttpms_1": 0, "rr_ttpms_1": 0}
        self.channel_ids = {"lf_ttpms_1": -1, "rf_ttpms_1": -1, "lr_ttpms_1": -1, "rr_ttpms_1": -1}

    def parse_tire_pressure(self) -> dict[str: str]:

        with open(self.mcap_file_path, "rb") as stream:
            reader = make_reader(stream, decoder_factories=[DecoderFactory()])
            try:
                for schema, channel, message, proto_msg in reader.iter_decoded_messages():

                    proto_msg_fields = proto_msg.ListFields()
                    if schema.name not in self.accepted_tire_topics:
                        continue

                    # When writing the code, the TTPMS_P (tire pressure) is the third element of the list in the entry
                    # Theoretically, the position of TTPMS_P in the list shouldn't change, but this accounts for if it does
                    if not proto_msg_fields[2][0].name.endswith("TTPMS_P"):
                        for field in proto_msg.ListFields():
                            if field[0].name.endswith("TTPMS_P"):
                                self.avg_pressures[schema.name] += float(field[1])
                                self.pressure_count[schema.name] += 1
                                self.channel_ids[schema.name] = channel.id
                                break
                    else:
                        self.avg_pressures[schema.name] += float(proto_msg_fields[2][1])
                        self.pressure_count[schema.name] += 1
                        self.channel_ids[schema.name] = channel.id

                for key, val in self.avg_pressures.items():
                    if self.pressure_count[key] != 0:
                        # The metadata for a mcap file takes a dict[str: str]
                        self.avg_pressures[key] = str(self.avg_pressures[key] / self.pressure_count[key])
            except mcap.exceptions.EndOfFile:
                print("Reached End of File")

            return self.avg_pressures

    def add_pressures_to_mcap(self):
        base, extension = os.path.splitext(self.mcap_file_path)
        base += "_V2"

        with open(base + extension, "wb") as f, Writer(f) as mcap_writer:
            # Rewriting all the messages from the original file into the new file.
            # This is because mcap files don't provide an easy way to edit files other than rewriting them
            with open(self.mcap_file_path, "rb") as stream_reader:
                reader = make_reader(stream_reader, decoder_factories=[DecoderFactory()])
                for schema, channel, message, proto_msg in reader.iter_decoded_messages():
                    mcap_writer.write_message(topic=channel.topic,
                                              message=proto_msg,
                                              log_time=message.log_time,
                                              publish_time=message.publish_time)

            # mcap_protobuf.writer is a higher-level abstraction of the mcap_writer class
            # So we have to access add_metadata through _writer
            mcap_writer._writer.add_metadata("TTPMS_P_AVG", self.avg_pressures)
            mcap_writer.finish()


handler = MCAPHandler("mcap_files/03_05_2024_23_10_23.mcap")
pressures = handler.parse_tire_pressure()
handler.add_pressures_to_mcap()
