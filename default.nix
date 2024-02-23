{ lib
, python311Packages
}:

python311Packages.buildPythonApplication {
  pname = "cloud_webserver";
  version = "1.0.0";

  propagatedBuildInputs = [
    python311Packages.pymongo
    python311Pachages.flask
  ];

  src = ./cloud_webserver;
}