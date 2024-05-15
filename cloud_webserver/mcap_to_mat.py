from pathlib import Path
from mcap.reader import make_reader
from mcap_protobuf.decoder import DecoderFactory
from scipy.io import savemat
 
def parser(input):
    with open(input, "rb") as f:
        reader = make_reader(f, decoder_factories=[DecoderFactory()])
        topics = []
        for channel in reader.get_summary().channels:
            topics.append(reader.get_summary().channels[channel].topic)
 
        mdict = {}
        for topic in topics:
            msg_dict = {}
            first_log_time = None  # Track the first log time for each topic
            for schema, channel, message, proto_msg in reader.iter_decoded_messages(topics=[topic]):
                res = [f.name for f in proto_msg.DESCRIPTOR.fields]
                for name in res:
                    if name not in msg_dict:
                        msg_dict[name] = []
                    log_time = message.log_time / 1e9  # Convert nanoseconds to seconds
                    if first_log_time is None:
                        first_log_time = log_time  # Set the first log time
                    signal_data = [log_time - first_log_time, int ( getattr(proto_msg, name) )]  # Subtract the first log time
                    msg_dict[name].append(signal_data)
                
            mdict[topic] = msg_dict
        mdict = {"data": mdict}

        file_name = Path(input).stem
        out_name = f"files/{file_name}"
        savemat(out_name+".mat", mdict, long_field_names=True)
        return f"{file_name}.mat"
