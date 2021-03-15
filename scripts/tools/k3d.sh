# shellcheck shell=bash
# shellcheck disable=SC2034

TOOLS+=(k3d)

K3D_VERSION="4.3.0"

function k3d_version { k3d version; }

K3D_SHA256_DARWIN="17925d18fb573932d57093afad0e772dce4b1181584a7a99fca19a58d478d9c5"
K3D_SHA256_LINUX="afb1a4b2df657c3721cc4034a381c05c3c7cc343198103157317d1e891f2ee7f"
K3D_SHA256_WINDOWS="a8434cb1d1314d0aaaeb62cb3c709e38ef5103f7ed790eca1ddacdb9ebf499d3"

K3D_URL_DARWIN="https://github.com/rancher/k3d/releases/download/v{version}/k3d-darwin-amd64"
K3D_URL_LINUX="https://github.com/rancher/k3d/releases/download/v{version}/k3d-linux-amd64"
K3D_URL_WINDOWS="https://github.com/rancher/k3d/releases/download/v{version}/k3d-windows-amd64.exe"
