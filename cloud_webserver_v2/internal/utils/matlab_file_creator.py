import sys
from scipy.io import savemat
import json

def main(file_path: str):
    try:
        input_data = sys.stdin.read()
        
        data = {"data": json.loads(input_data)}
        # data = json.loads(input_data)

        # Attempt to save the data as .mat
        savemat(file_path, data, long_field_names=True)
        print("MATLAB file created successfully.")

    except json.JSONDecodeError as e:
        print("Error decoding JSON input:", e)
    except Exception as e:
        print("An error occurred:", e)


if __name__ == '__main__':
    file_path = sys.argv[1]
    main(file_path)

