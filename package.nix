#based on default.nix in HT_CAN
{ pkgs }:

pkgs.stdenv.mkDerivation rec {
  name = "yaml_pkg";
  src = ./docker;
  
 

  # Specify the output of the build process
  # In this case, it will be the generated file
  installPhase = ''
    mkdir -p $out
    mv docker-compose.yml $out/docker-compose.yml
  '';
}