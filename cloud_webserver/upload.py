import os
from flask import Flask, flash, request, redirect, url_for
from werkzeug.datastructures import FileStorage
from werkzeug.utils import secure_filename

UPLOAD_FOLDER: str = 'files'
ALLOWED_EXTENSIONS = {'mcap', 'mat'}


def allowed_file(filename: str) -> bool:
    return '.' in filename and filename.rsplit('.', 1)[1].lower() in ALLOWED_EXTENSIONS


def new_file_path(curr_filename: str) -> str:
    secure: str = secure_filename(curr_filename)
    if not os.path.exists(os.path.join(UPLOAD_FOLDER, secure)):
        return secure

    filename: str = curr_filename.rsplit('.', 1)[0]
    ext: str = curr_filename.rsplit('.', 1)[1]

    low: int = 0
    high: int = (1 << 30)
    while low < high:
        mid: int = low + (high-low)//2
        if os.path.exists(os.path.join(UPLOAD_FOLDER, filename + '(' + str(mid) + ').' + ext)):
            low = mid + 1
        else:
            high = mid

    return filename + '(' + str(low) + ').' + ext


def save_mcap_file(file: FileStorage) -> str:
    if not file:
        raise ValueError('File does not exist')
    if not allowed_file(file.filename):
        raise ValueError('Illegal filetype')

    if file and allowed_file(file.filename):
        if not os.path.isdir(UPLOAD_FOLDER):
            os.mkdir(UPLOAD_FOLDER)
        path_to_file: str = os.path.join(UPLOAD_FOLDER, new_file_path(file.filename))
        file.save(path_to_file)

        return path_to_file
    return ""
