{ lib
, python311Packages
, py_mcap_pkg
, mcap_support_pkg
}:

python311Packages.buildPythonApplication {
  pname = "cloud_webserver";
  version = "1.0.0";

  propagatedBuildInputs = [
    python311Packages.pymongo
    python311Packages.flask
    mcap_support_pkg
    py_mcap_pkg
    python311Packages.werkzeug
    python311Packages.lz4
    python311Packages.zstandard
  ];

  src = ./cloud_webserver;
}