import json
from time import time_ns

import mcap.exceptions
from mcap.decoder import DecoderFactory
from mcap.writer import Writer
from mcap_protobuf.decoder import DecoderFactory
from mcap.reader import make_reader

class MCAPHandler():
    def __init__(self, mcap_file_path):
        self.mcap_file_path = mcap_file_path
        self.accepted_topics = ["lf_ttpms_1", "rf_ttpms_1", "lr_ttpms_1", "rr_ttpms_1"]
        self.avg_pressures = {"lf_ttpms_1": 0, "rf_ttpms_1": 0, "lr_ttpms_1": 0, "rr_ttpms_1": 0}
        self.pressure_count = {"lf_ttpms_1": 0, "rf_ttpms_1": 0, "lr_ttpms_1": 0, "rr_ttpms_1": 0}
        self.channel_ids = {"lf_ttpms_1": -1, "rf_ttpms_1": -1, "lr_ttpms_1": -1, "rr_ttpms_1": -1}

    def parse_tire_pressure(self) -> dict[str: float]:

        with open(self.mcap_file_path, "rb") as stream:
            reader = make_reader(stream, decoder_factories=[DecoderFactory()])
            try:
                for schema, channel, message, proto_msg in reader.iter_decoded_messages():

                    proto_msg_fields = proto_msg.ListFields()
                    if schema.name not in self.accepted_topics:
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
                        self.avg_pressures[key] = self.avg_pressures[key] / self.pressure_count[key]
            except mcap.exceptions.EndOfFile:
                print("Reached End of File")

            return self.avg_pressures

    def add_pressures_to_mcap(self):
        with open(self.mcap_file_path, "ab") as stream:
            writer = Writer(stream)
            writer.start()

            for name, channel_id in self.channel_ids.items():
                if channel_id != -1:
                    writer.add_message(
                        channel_id=channel_id,
                        log_time=time_ns(),
                        data=json.dumps({str.upper(name[0:2]) + "TTPMS_P_AVG": self.avg_pressures[name]}).encode("UTF-8"),
                        publish_time=time_ns(),
                    )
            writer.finish()



handler = MCAPHandler("mcap_files/02_29_2024_20_17_28.mcap")
pressures = handler.parse_tire_pressure()
print(pressures)
handler.add_pressures_to_mcap()